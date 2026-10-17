// Package tz 解开 Topaz 的 .tz 模型文件。
//
// .tz 是一个用传统 ZipCrypto（而非 AES）加密的 ZIP 包，内含单个 ONNX 模型。
// 本包实现 ZipCrypto 解密 + deflate 解压，直接产出 ONNX 字节，不依赖外部工具。
package tz

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"io"
)

// Password 是 aiengine.dll 中硬编码的 .tz 解包密码。
const Password = "a0331add513e4e6fae035cb12e4b11e5"

// zipCrypto 实现 WinZip 传统加密的流密码。
type zipCrypto struct {
	key0, key1, key2 uint32
}

func (z *zipCrypto) updateKeys(b byte) {
	z.key0 = crc32Byte(z.key0, b)
	z.key1 = (z.key1+uint32(z.key0&0xff))*134775813 + 1
	z.key2 = crc32Byte(z.key2, byte(z.key1>>24))
}

func (z *zipCrypto) decryptByte() byte {
	temp := uint16(z.key2) | 2
	return byte((temp * (temp ^ 1)) >> 8)
}

func (z *zipCrypto) init(password []byte) {
	z.key0, z.key1, z.key2 = 0x12345678, 0x23456789, 0x34567890
	for _, c := range password {
		z.updateKeys(c)
	}
}

// crc32Byte 用查表法计算 CRC32（多项式 0xEDB88320），逐字节更新。
func crc32Byte(crc uint32, b byte) uint32 {
	crc ^= uint32(b)
	for range 8 {
		if crc&1 != 0 {
			crc = (crc >> 1) ^ 0xEDB88320
		} else {
			crc >>= 1
		}
	}
	return crc
}

// Extract 从 .tz 数据中解出 ONNX 内容。返回解压后的字节。
func Extract(data []byte, password string) ([]byte, error) {
	// 先定位中央目录结尾记录（EOCD），以获取中央目录偏移。
	eocd := findEOCD(data)
	if eocd < 0 {
		return nil, fmt.Errorf("tz: 未找到 ZIP 中央目录结尾")
	}
	cdOffset := binary.LittleEndian.Uint32(data[eocd+16:])
	cdSize := binary.LittleEndian.Uint32(data[eocd+12:])
	_ = cdSize

	pos := int(cdOffset)
	if int(pos)+46 > len(data) {
		return nil, fmt.Errorf("tz: 中央目录偏移越界")
	}
	if binary.LittleEndian.Uint32(data[pos:]) != 0x02014b50 {
		return nil, fmt.Errorf("tz: 中央目录签名错误")
	}
	flags := binary.LittleEndian.Uint16(data[pos+8:])
	method := binary.LittleEndian.Uint16(data[pos+10:])
	nameLen := int(binary.LittleEndian.Uint16(data[pos+28:]))
	localOffset := binary.LittleEndian.Uint32(data[pos+42:])
	name := string(data[pos+46 : pos+46+nameLen])

	// 跳到本地文件头，读取实际数据位置。
	lp := int(localOffset)
	if lp+30 > len(data) || binary.LittleEndian.Uint32(data[lp:]) != 0x04034b50 {
		return nil, fmt.Errorf("tz: 本地文件头签名错误")
	}
	lNameLen := binary.LittleEndian.Uint16(data[lp+26:])
	lExtraLen := binary.LittleEndian.Uint16(data[lp+28:])
	dataStart := lp + 30 + int(lNameLen) + int(lExtraLen)

	// 中央目录里记录的压缩大小即加密后的数据长度。
	compSize := int(binary.LittleEndian.Uint32(data[pos+20:]))
	if dataStart+compSize > len(data) {
		compSize = len(data) - dataStart
	}
	enc := data[dataStart : dataStart+compSize]

	encrypted := flags&1 != 0
	var plain []byte
	if encrypted {
		if len(enc) < 12 {
			return nil, fmt.Errorf("tz: 加密数据过短")
		}
		var c zipCrypto
		c.init([]byte(password))
		// 解密 12 字节加密头（前 11 字节为随机，最后 1 字节为校验）。
		hdr := make([]byte, 12)
		for i := range 12 {
			p := enc[i] ^ c.decryptByte()
			c.updateKeys(p)
			hdr[i] = p
		}
		plain = make([]byte, 0, compSize-12)
		rest := enc[12:]
		for _, b := range rest {
			p := b ^ c.decryptByte()
			c.updateKeys(p)
			plain = append(plain, p)
		}
	} else {
		plain = enc
	}

	switch method {
	case 0:
		return plain, nil
	case 8:
		r := flate.NewReader(bytes.NewReader(plain))
		defer r.Close()
		out, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("tz: 解压 %s 失败: %w", name, err)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("tz: 不支持的压缩方法 %d", method)
	}
}

func findEOCD(data []byte) int {
	// EOCD 至少 22 字节，注释最长 65535。从末尾向前找。
	limit := len(data) - 22
	if limit < 0 {
		return -1
	}
	start := max(len(data)-22-65535, 0)
	for i := limit; i >= start; i-- {
		if binary.LittleEndian.Uint32(data[i:]) == 0x06054b50 {
			return i
		}
	}
	return -1
}

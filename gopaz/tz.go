package main

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"io"
)

// Password is the .tz password hard-coded in aiengine.dll.
const Password = "a0331add513e4e6fae035cb12e4b11e5"

// zipCrypto implements the WinZip traditional encryption stream cipher.
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

// crc32Byte updates CRC32 with the 0xEDB88320 polynomial for one byte.
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

// Extract decompresses the ONNX payload from .tz data.
func Extract(data []byte, password string) ([]byte, error) {
	eocd := findEOCD(data)
	if eocd < 0 {
		return nil, fmt.Errorf("tz: ZIP end of central directory not found")
	}
	cdOffset := binary.LittleEndian.Uint32(data[eocd+16:])
	cdSize := binary.LittleEndian.Uint32(data[eocd+12:])
	_ = cdSize

	pos := int(cdOffset)
	if int(pos)+46 > len(data) {
		return nil, fmt.Errorf("tz: central directory offset out of bounds")
	}
	if binary.LittleEndian.Uint32(data[pos:]) != 0x02014b50 {
		return nil, fmt.Errorf("tz: bad central directory signature")
	}
	flags := binary.LittleEndian.Uint16(data[pos+8:])
	method := binary.LittleEndian.Uint16(data[pos+10:])
	nameLen := int(binary.LittleEndian.Uint16(data[pos+28:]))
	localOffset := binary.LittleEndian.Uint32(data[pos+42:])
	name := string(data[pos+46 : pos+46+nameLen])

	lp := int(localOffset)
	if lp+30 > len(data) || binary.LittleEndian.Uint32(data[lp:]) != 0x04034b50 {
		return nil, fmt.Errorf("tz: bad local file header signature")
	}
	lNameLen := binary.LittleEndian.Uint16(data[lp+26:])
	lExtraLen := binary.LittleEndian.Uint16(data[lp+28:])
	dataStart := lp + 30 + int(lNameLen) + int(lExtraLen)

	compSize := int(binary.LittleEndian.Uint32(data[pos+20:]))
	if dataStart+compSize > len(data) {
		compSize = len(data) - dataStart
	}
	enc := data[dataStart : dataStart+compSize]

	encrypted := flags&1 != 0
	var plain []byte
	if encrypted {
		if len(enc) < 12 {
			return nil, fmt.Errorf("tz: encrypted payload is too short")
		}
		var c zipCrypto
		c.init([]byte(password))
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
			return nil, fmt.Errorf("tz: failed to decompress %s: %w", name, err)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("tz: unsupported compression method %d", method)
	}
}

func findEOCD(data []byte) int {
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

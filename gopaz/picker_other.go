//go:build !windows

package main

import "errors"

func pickVideoFile() (string, error) {
	return "", errors.New("native file picker is only available on Windows")
}

func pickOutputFolder() (string, error) {
	return "", errors.New("native folder picker is only available on Windows")
}

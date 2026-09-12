package filechat

import (
	"context"
	"errors"
	"io"
	"os"
)

const (
	indexPrefixHashOffset64 = uint64(14695981039346656037)
	indexPrefixHashPrime64  = uint64(1099511628211)
)

func updateIndexPrefixHash(value uint64, data []byte) uint64 {
	for _, item := range data {
		value ^= uint64(item)
		value *= indexPrefixHashPrime64
	}
	return value
}

func chatIndexPrefixMatches(
	ctx context.Context,
	eventsPath string,
	state chatIndexState,
) (bool, error) {
	file, err := os.Open(eventsPath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	value := indexPrefixHashOffset64
	remaining := state.indexedBytes
	buffer := make([]byte, 32*1024)
	for remaining > 0 {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		readSize := int64(len(buffer))
		if remaining < readSize {
			readSize = remaining
		}
		count, readErr := io.ReadFull(file, buffer[:int(readSize)])
		if count > 0 {
			value = updateIndexPrefixHash(value, buffer[:count])
			remaining -= int64(count)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) {
				return false, nil
			}
			return false, readErr
		}
	}
	return value == state.prefixHash, nil
}

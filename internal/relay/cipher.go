package relay

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

type aesCipher struct {
	block cipher.Block
}

func newCipher(key string) (*aesCipher, error) {
	if key == "" {
		return nil, nil
	}
	hash := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(hash[:])
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	return &aesCipher{block: block}, nil
}

func (c *aesCipher) encrypt(plaintext []byte) []byte {
	if c == nil {
		return plaintext
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return plaintext
	}
	stream := cipher.NewCTR(c.block, iv)
	ciphertext := make([]byte, len(plaintext))
	stream.XORKeyStream(ciphertext, plaintext)
	result := make([]byte, aes.BlockSize+len(plaintext))
	copy(result[:aes.BlockSize], iv)
	copy(result[aes.BlockSize:], ciphertext)
	return result
}

func (c *aesCipher) decrypt(data []byte) ([]byte, error) {
	if c == nil {
		return data, nil
	}
	if len(data) < aes.BlockSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	iv := data[:aes.BlockSize]
	stream := cipher.NewCTR(c.block, iv)
	plaintext := make([]byte, len(data)-aes.BlockSize)
	stream.XORKeyStream(plaintext, data[aes.BlockSize:])
	return plaintext, nil
}

func (c *aesCipher) enabled() bool {
	return c != nil
}

func (c *aesCipher) blockSize() int {
	if c == nil {
		return 0
	}
	return aes.BlockSize
}

func readU16BE(b []byte) uint16 {
	return binary.BigEndian.Uint16(b)
}

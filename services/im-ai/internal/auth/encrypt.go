package auth

import (
	"bytes"
	"crypto/aes"
	"encoding/base64"
	"errors"
)

func pkcs5Padding(src []byte, blockSize int) []byte {
	padding := blockSize - len(src)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(src, padtext...)
}

func pkcs5UnPadding(src []byte) ([]byte, error) {
	length := len(src)
	if length == 0 {
		return nil, errors.New("invalid padding size")
	}
	unpadding := int(src[length-1])
	if unpadding <= 0 || unpadding > length {
		return nil, errors.New("invalid padding")
	}
	// verify padding bytes
	for i := 0; i < unpadding; i++ {
		if src[length-1-i] != byte(unpadding) {
			return nil, errors.New("invalid padding")
		}
	}
	return src[:(length - unpadding)], nil
}

// Encrypt corresponds to Java EncryptUtils.encrypt:
// AES/ECB/PKCS5Padding, key = secret UTF-8 bytes, output = Base64.
func Encrypt(content, secret string) (string, error) {
	key := []byte(secret)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	bs := block.BlockSize()
	origData := pkcs5Padding([]byte(content), bs)

	encrypted := make([]byte, len(origData))
	for i := 0; i < len(origData); i += bs {
		block.Encrypt(encrypted[i:i+bs], origData[i:i+bs])
	}

	return base64.StdEncoding.EncodeToString(encrypted), nil
}

// Decrypt corresponds to Java EncryptUtils.decrypt.
func Decrypt(content, secret string) (string, error) {
	key := []byte(secret)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	encrypted, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return "", err
	}
	if len(encrypted) == 0 || len(encrypted)%block.BlockSize() != 0 {
		return "", errors.New("invalid ciphertext size")
	}

	decrypted := make([]byte, len(encrypted))
	bs := block.BlockSize()
	for i := 0; i < len(encrypted); i += bs {
		block.Decrypt(decrypted[i:i+bs], encrypted[i:i+bs])
	}

	decrypted, err = pkcs5UnPadding(decrypted)
	if err != nil {
		return "", err
	}
	return string(decrypted), nil
}

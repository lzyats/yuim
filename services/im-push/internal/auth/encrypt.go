package auth

import (
	"crypto/aes"
	"encoding/base64"
	"errors"
)

// Decrypt is compatible with Java EncryptUtils.decrypt:
// Cipher = AES/ECB/PKCS5Padding, key = secret UTF-8 bytes, output/input = Base64.
func Decrypt(contentBase64, secret string) (string, error) {
	key := []byte(secret)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	enc, err := base64.StdEncoding.DecodeString(contentBase64)
	if err != nil {
		return "", err
	}
	if len(enc) == 0 || len(enc)%block.BlockSize() != 0 {
		return "", errors.New("invalid ciphertext size")
	}

	bs := block.BlockSize()
	out := make([]byte, len(enc))
	for i := 0; i < len(enc); i += bs {
		block.Decrypt(out[i:i+bs], enc[i:i+bs])
	}
	unpadded, err := pkcs5UnPadding(out)
	if err != nil {
		return "", err
	}
	return string(unpadded), nil
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
	return src[:length-unpadding], nil
}

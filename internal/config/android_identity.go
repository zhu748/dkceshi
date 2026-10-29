package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

// AndroidInstallationID follows APK 2.7.1 Lov3.a/b. It is local installation
// identity, not a replacement for a risk SDK ID or device attestation.
func AndroidInstallationID(acc Account) string {
	if id := strings.TrimSpace(acc.InstallationID); id != "" {
		return id
	}
	id := strings.TrimSpace(acc.AndroidID)
	if id == "" {
		sum := sha256.Sum256([]byte("ds2api/android-id/" + strings.ToLower(acc.Identifier())))
		id = hex.EncodeToString(sum[:8])
	}
	brand := strings.TrimSpace(acc.DeviceBrand)
	if brand == "" {
		brand = AndroidBrandForModel(acc.DeviceModel)
	}
	plain := []byte(id + "_" + brand)
	padding := aes.BlockSize - len(plain)%aes.BlockSize
	for range padding {
		plain = append(plain, byte(padding))
	}
	key := md5.Sum([]byte("com.deepseek.chat"))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		panic(err) // fixed 16-byte AES key
	}
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(plain, plain)
	return base64.StdEncoding.EncodeToString(plain)
}

func AndroidBrandForModel(model string) string {
	switch {
	case strings.HasPrefix(model, "Pixel"):
		return "google"
	case strings.HasPrefix(model, "SM-"):
		return "samsung"
	case strings.HasPrefix(model, "V"):
		return "vivo"
	default:
		return "android"
	}
}

func cloneAccounts(in []Account) []Account {
	if in == nil {
		return nil
	}
	out := make([]Account, len(in))
	for i, acc := range in {
		out[i] = acc.clone()
	}
	return out
}

func (a Account) clone() Account {
	if a.PhotoCompressRatio != nil {
		ratio := *a.PhotoCompressRatio
		a.PhotoCompressRatio = &ratio
	}
	return a
}

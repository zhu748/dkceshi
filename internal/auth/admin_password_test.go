package auth

import (
	"strings"
	"testing"
)

func TestGenerateAdminPassword_LengthAndCharset(t *testing.T) {
	p, err := GenerateAdminPassword()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(p) < 16 {
		t.Fatalf("password too short: %d", len(p))
	}
	// 只含安全字符集（字母数字，无易混淆字符）
	for _, r := range p {
		if !strings.ContainsRune(adminPasswordCharset, r) {
			t.Fatalf("unexpected char: %c", r)
		}
	}
}

func TestGenerateAdminPassword_Randomness(t *testing.T) {
	a, err := GenerateAdminPassword()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	b, err := GenerateAdminPassword()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if a == b {
		t.Fatalf("two generated passwords identical")
	}
}

func TestPasswordsAreSaltedAndLegacyHashesStillVerify(t *testing.T) {
	a, b := HashAdminPassword("correct-password"), HashAdminPassword("correct-password")
	if a == b || !strings.HasPrefix(a, passwordHashPrefix) {
		t.Fatal("new hashes must use a random salt")
	}
	if !verifyAdminPasswordHash("correct-password", a) || verifyAdminPasswordHash("wrong", a) {
		t.Fatal("salted verification failed")
	}
	// SHA-256("password"), used by existing installations.
	if !verifyAdminPasswordHash("password", "sha256:5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8") {
		t.Fatal("legacy password stopped working")
	}
	long := strings.Repeat("长密码", 40)
	if !verifyAdminPasswordHash(long, HashAdminPassword(long)) {
		t.Fatal("long UTF-8 passwords were truncated")
	}
}

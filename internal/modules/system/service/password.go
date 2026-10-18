package service

import (
	"golang.org/x/crypto/bcrypt"

	"gowms/internal/pkg/errcode"
)

func hashPassword(plain string) (string, error) {
	if len(plain) > 72 {
		return "", errcode.PasswordTooLong
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

func checkPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

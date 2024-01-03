package main

import (
	"errors"
	"os"
)

type Command interface {
	Execute(*SecureVault) error
}

type CreateCommand struct{}

func (c *CreateCommand) Execute(vault *SecureVault) error {
	password := readPassword("Enter pass phrase to secure safe vault:")
	password2 := readPassword("Reenter pass phrase to secure safe vault:")
	if string(password) != string(password2) {
		return errors.New("Passwords do not match")
	}
	vault = NewSecureVault([]byte(password), os.Args[1])
	if vault == nil {
		return errors.New("Could not create vault")
	}
	return nil
}

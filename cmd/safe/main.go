package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/freehandle/breeze/util"
	"golang.org/x/term"
)

var usage = `Usage: 

	safe <path-to-vault-file> <command> [arguments] 

The commands are:

	create    create new vault file
	register  register trsuted node on breeze network
	remove    remove trusted node from breeze network
	nodes     list all trusted nodes on breeze network
	generate  generate new random key pair and secure them on vault
	
Use "safe help <command>" for more information about a command.

`

func ParseCommand(params ...string) {
	if len(params) == 0 {
		return
	}
	if params[0] == "help" {
		if len(params) > 1 {
			if params[1] == "new" {
				fmt.Print(helpNew)
				os.Exit(0)
			}
		}
	}
}

func main() {
	ParseCommand(os.Args[1:]...)
	if len(os.Args) < 2 {
		fmt.Println(usage)
		return
	}
	var safe *util.SecureVault
	if stat, _ := os.Stat(os.Args[1]); stat == nil {
		fmt.Print("File does not exist. Create new [yes/no]?")
		var yes string
		fmt.Scan(&yes)
		yes = strings.TrimSpace(strings.ToLower(yes))
		if yes == "yes" || yes == "y" {
			fmt.Println("Enter pass phrase to secure safe vault:")
			password, err := term.ReadPassword(0)
			if err != nil {
				fmt.Printf("Error reading password: %v\n", err)
				return
			}
			safe = util.NewSecureVault([]byte(password), os.Args[1])
			if safe == nil {
				fmt.Println("Could not create vault")
				return
			}
		} else {
			return
		}
	} else if stat.IsDir() {
		fmt.Println("File is a directory")
		return
	} else {
		fmt.Println("Enter pass phrase:")
		password, err := term.ReadPassword(0)
		if err != nil {
			fmt.Printf("Error reading password: %v\n", err)
			return
		}
		safe = util.OpenVaultFromPassword([]byte(password), os.Args[1])
		if safe == nil {
			fmt.Println("Could not open vault")
			return
		}
	}
}

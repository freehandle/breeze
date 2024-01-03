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

const (
	noCmd byte = iota
	createCmd
	registerCmd
	removeCmd
	nodesCmd
	generateCmd
)

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

func readPassword(phrase string) []byte {
	fmt.Println(phrase)
	password, err := term.ReadPassword(0)
	for {
		if err != nil {
			fmt.Printf("Error reading password: %v\n", err)
			os.Exit(1)
		}
		if len(password) == 0 {
			fmt.Printf("Try again:\n")
		} else {
			break
		}
	}
	return password
}

func create() {

}

func yesorno(caption string) bool {
	var yes string
	fmt.Print(caption)
	fmt.Scan(&yes)
	yes = strings.TrimSpace(strings.ToLower(yes))
	return yes == "yes" || yes == "y"
}

func parseCommand() byte {
	if len(os.Args) < 3 {
		return noCmd
	}
	switch strings.ToLower(os.Args[2]) {
	case "create":
		return createCmd
	case "register":
		return registerCmd
	case "remove":
		return removeCmd
	case "nodes":
		return nodesCmd
	case "generate":
		return generateCmd
	default:
		return noCmd
	}
}

func register()

func main() {
	ParseCommand(os.Args[1:]...)
	if len(os.Args) < 2 {
		fmt.Println(usage)
		return
	}
	var safe *util.SecureVault
	cmd := parseCommand()
	if stat, _ := os.Stat(os.Args[1]); stat == nil {
		if cmd == createCmd || yesorno("File does not exist. Create new [yes/no]?") {
			create()
		} else {
			return
		}
	} else if stat.IsDir() {
		fmt.Println("File is a directory")
		return
	} else {
		if cmd == createCmd {
			fmt.Println("File already exists")
			return
		}
		password := readPassword("Enter pass phrase to open vault:")
		safe = util.OpenVaultFromPassword([]byte(password), os.Args[1])
		if safe == nil {
			fmt.Println("Could not open vault")
			return
		}
	}
	if cmd == noCmd {
		fmt.Println("done")
		return
	}
}

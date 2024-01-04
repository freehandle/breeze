package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

var usage = `Usage: 

	safe <path-to-vault-file> <command> [arguments] 

The commands are:

	create    create new vault file
	register  register trusted node on breeze network
	remove    remove trusted node from breeze network
	nodes     list all trusted nodes on breeze network
	generate  generate new random key pair and secure them on vault
	sync      sync required keys with trusted node
	grant	  grant token with access to connect as a gateway or block listener
	revoke    revoke token access to connect as a gateway or block listener
	activity  instructs trusted node whether to candidate to become a validator. 
	status    show status of a trusted node
	transfer  transfer tokens between accounts
	deposit	  deposit tokens in account
	withdraw  withdraw tokens from account
	balance	  get token balanece information of given account
	
Use "safe help <command>" for more information about a command.

`

const (
	noCmd byte = iota
	createCmd
	registerCmd
	removeCmd
	nodesCmd
	generateCmd
	syncCmd
	grantCmd
	revokeCmd
	activityCmd
	statusCmd
	transferCmd
	depositCmd
	withdrawCmd
	balanceCmd
	configCmd
)

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
	case "sync":
		return syncCmd
	case "grant":
		return grantCmd
	case "revoke":
		return revokeCmd
	case "activity":
		return activityCmd
	case "status":
		return statusCmd
	case "transfer":
		return transferCmd
	case "deposit":
		return depositCmd
	case "withdraw":
		return withdrawCmd
	case "balance":
		return balanceCmd
	case "config":
		return configCmd
	default:
		return noCmd
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println(usage)
		return
	}
	if strings.ToLower(os.Args[1]) == "help" {
		if len(os.Args) < 3 {
			fmt.Println(usage)
		} else {
			help(os.Args[2])
		}
		return
	}

	var safe *Safe
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
		var err error
		safe, err = OpenVaultFromPassword([]byte(password), os.Args[1])
		if safe == nil {
			fmt.Printf("Could not open vault: %v\n", err)
			return
		}
	}
	if cmd == noCmd {
		fmt.Println("done")
		return
	}
	execution := parseCommandArgs(cmd, os.Args[3:])
	if execution == nil {
		fmt.Println("Invalid command")
		return
	}
	err := execution.Execute(safe)
	if err != nil {
		fmt.Printf("Error executing command: %v\n", err)
		return
	}
}

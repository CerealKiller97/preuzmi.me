package cmd

import (
	"errors"
	"fmt"
	"github.com/CerealKiller97/preuzmi.me/pkg/container"
	"os"
)

const (
	serve  Command = "serve"
	checks Command = "checks"
	help   Command = "help"
)

var (
	commands = []Command{
		serve,
		checks,
		help,
	}

	ErrNoCommandSpecified = errors.New("you must specify a command to run")
)

type (
	Command string
)

func (c Command) Usage() string {
	desc := ""

	switch c {
	case serve:
		desc = "Start an HTTP server with the configured parameters"
	case checks:
		desc = "Checks that you have the configured parameters"
	case help:
		desc = "Print this help dialog"
	}

	return fmt.Sprintf("  %s\n    %s\n", c, desc)
}

// usage prints out the list of available options
func usage() {
	name := os.Args[0]
	usage := "Copyright (C) 2025 Stefan Bogdanović <stefan@stefanbogdanovic.dev>\n" +
		"Licensed under the terms of the GNU AGPL v3 only\n\n" +
		"Preuzmi.me a tool for downloading receipts \n\n" +
		"Usage of %s [COMMAND] [FLAGS]\n\n" +
		"Commands:\n"

	for _, command := range commands {
		usage += command.Usage()
	}

	fmt.Printf(usage, name)
}

// get takes the first argument
// and treats it as the command name
// while performing bounds checking
func get() (Command, error) {
	if len(os.Args) == 1 {
		return "", ErrNoCommandSpecified
	}

	return Command(os.Args[1]), nil
}

func Run(c *container.Container) {
	command, err := get()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n\n", err.Error())
		usage()

		os.Exit(1)
	}

	switch command {
	case serve:
		Serve(c)
	case help:
		usage()
	case checks:
		Checks(c)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command \"%s\"\n\n", command)
		usage()
	}
}

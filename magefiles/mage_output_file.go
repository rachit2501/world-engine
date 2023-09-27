// +build ignore

package main

import (
	"context"
	_flag "flag"
	_fmt "fmt"
	_ioutil "io/ioutil"
	_log "log"
	"os"
	"os/signal"
	_filepath "path/filepath"
	_sort "sort"
	"strconv"
	_strings "strings"
	"syscall"
	_tabwriter "text/tabwriter"
	"time"
	
)

func main() {
	// Use local types and functions in order to avoid name conflicts with additional magefiles.
	type arguments struct {
		Verbose       bool          // print out log statements
		List          bool          // print out a list of targets
		Help          bool          // print out help for a specific target
		Timeout       time.Duration // set a timeout to running the targets
		Args          []string      // args contain the non-flag command-line arguments
	}

	parseBool := func(env string) bool {
		val := os.Getenv(env)
		if val == "" {
			return false
		}		
		b, err := strconv.ParseBool(val)
		if err != nil {
			_log.Printf("warning: environment variable %s is not a valid bool value: %v", env, val)
			return false
		}
		return b
	}

	parseDuration := func(env string) time.Duration {
		val := os.Getenv(env)
		if val == "" {
			return 0
		}		
		d, err := time.ParseDuration(val)
		if err != nil {
			_log.Printf("warning: environment variable %s is not a valid duration value: %v", env, val)
			return 0
		}
		return d
	}
	args := arguments{}
	fs := _flag.FlagSet{}
	fs.SetOutput(os.Stdout)

	// default flag set with ExitOnError and auto generated PrintDefaults should be sufficient
	fs.BoolVar(&args.Verbose, "v", parseBool("MAGEFILE_VERBOSE"), "show verbose output when running targets")
	fs.BoolVar(&args.List, "l", parseBool("MAGEFILE_LIST"), "list targets for this binary")
	fs.BoolVar(&args.Help, "h", parseBool("MAGEFILE_HELP"), "print out help for a specific target")
	fs.DurationVar(&args.Timeout, "t", parseDuration("MAGEFILE_TIMEOUT"), "timeout in duration parsable format (e.g. 5m30s)")
	fs.Usage = func() {
		_fmt.Fprintf(os.Stdout, `
%s [options] [target]

Commands:
  -l    list targets in this binary
  -h    show this help

Options:
  -h    show description of a target
  -t <string>
        timeout in duration parsable format (e.g. 5m30s)
  -v    show verbose output when running targets
 `[1:], _filepath.Base(os.Args[0]))
	}
	if err := fs.Parse(os.Args[1:]); err != nil {
		// flag will have printed out an error already.
		return
	}
	args.Args = fs.Args()
	if args.Help && len(args.Args) == 0 {
		fs.Usage()
		return
	}
		
	// color is ANSI color type
	type color int

	// If you add/change/remove any items in this constant,
	// you will need to run "stringer -type=color" in this directory again.
	// NOTE: Please keep the list in an alphabetical order.
	const (
		black color = iota
		red
		green
		yellow
		blue
		magenta
		cyan
		white
		brightblack
		brightred
		brightgreen
		brightyellow
		brightblue
		brightmagenta
		brightcyan
		brightwhite
	)

	// AnsiColor are ANSI color codes for supported terminal colors.
	var ansiColor = map[color]string{
		black:         "\u001b[30m",
		red:           "\u001b[31m",
		green:         "\u001b[32m",
		yellow:        "\u001b[33m",
		blue:          "\u001b[34m",
		magenta:       "\u001b[35m",
		cyan:          "\u001b[36m",
		white:         "\u001b[37m",
		brightblack:   "\u001b[30;1m",
		brightred:     "\u001b[31;1m",
		brightgreen:   "\u001b[32;1m",
		brightyellow:  "\u001b[33;1m",
		brightblue:    "\u001b[34;1m",
		brightmagenta: "\u001b[35;1m",
		brightcyan:    "\u001b[36;1m",
		brightwhite:   "\u001b[37;1m",
	}
	
	const _color_name = "blackredgreenyellowbluemagentacyanwhitebrightblackbrightredbrightgreenbrightyellowbrightbluebrightmagentabrightcyanbrightwhite"

	var _color_index = [...]uint8{0, 5, 8, 13, 19, 23, 30, 34, 39, 50, 59, 70, 82, 92, 105, 115, 126}

	colorToLowerString := func (i color) string {
		if i < 0 || i >= color(len(_color_index)-1) {
			return "color(" + strconv.FormatInt(int64(i), 10) + ")"
		}
		return _color_name[_color_index[i]:_color_index[i+1]]
	}

	// ansiColorReset is an ANSI color code to reset the terminal color.
	const ansiColorReset = "\033[0m"

	// defaultTargetAnsiColor is a default ANSI color for colorizing targets.
	// It is set to Cyan as an arbitrary color, because it has a neutral meaning
	var defaultTargetAnsiColor = ansiColor[cyan]

	getAnsiColor := func(color string) (string, bool) {
		colorLower := _strings.ToLower(color)
		for k, v := range ansiColor {
			colorConstLower := colorToLowerString(k)
			if colorConstLower == colorLower {
				return v, true
			}
		}
		return "", false
	}

	// Terminals which  don't support color:
	// 	TERM=vt100
	// 	TERM=cygwin
	// 	TERM=xterm-mono
    var noColorTerms = map[string]bool{
		"vt100":      false,
		"cygwin":     false,
		"xterm-mono": false,
	}

	// terminalSupportsColor checks if the current console supports color output
	//
	// Supported:
	// 	linux, mac, or windows's ConEmu, Cmder, putty, git-bash.exe, pwsh.exe
	// Not supported:
	// 	windows cmd.exe, powerShell.exe
	terminalSupportsColor := func() bool {
		envTerm := os.Getenv("TERM")
		if _, ok := noColorTerms[envTerm]; ok {
			return false
		}
		return true
	}

	// enableColor reports whether the user has requested to enable a color output.
	enableColor := func() bool {
		b, _ := strconv.ParseBool(os.Getenv("MAGEFILE_ENABLE_COLOR"))
		return b
	}

	// targetColor returns the ANSI color which should be used to colorize targets.
	targetColor := func() string {
		s, exists := os.LookupEnv("MAGEFILE_TARGET_COLOR")
		if exists == true {
			if c, ok := getAnsiColor(s); ok == true {
				return c
			}
		}
		return defaultTargetAnsiColor
	}

	// store the color terminal variables, so that the detection isn't repeated for each target
	var enableColorValue = enableColor() && terminalSupportsColor()
	var targetColorValue = targetColor()

	printName := func(str string) string {
		if enableColorValue {
			return _fmt.Sprintf("%s%s%s", targetColorValue, str, ansiColorReset)
		} else {
			return str
		}
	}

	list := func() error {
		_fmt.Println(`SPDX-License-Identifier: MIT  # Copyright (c) 2023 Berachain Foundation  Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:  The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
`)
		targets := map[string]string{
			"all": "Runs a series of commonly used commands.",
			"build": "builds the project.",
			"clean": "Cleans the project.",
			"contracts:build": "Runs `forge build` in all smart contract directories.",
			"contracts:buildCheck": "Check that the generated forge build source files are up to date.",
			"contracts:clean": "Run `forge clean` in all smart contract directories.",
			"contracts:fmt": "Run `forge fmt` in all smart contract directories.",
			"contracts:test": "Run `forge test` in all smart contract directories.",
			"contracts:testIntegration": "",
			"contracts:testUnit": "Run `forge test` in all smart contract directories.",
			"cosmos:build": "builds the Cosmos SDK chain.",
			"cosmos:buildDocker": "Builds a release version of the Cosmos SDK chain.",
			"cosmos:buildDockerDebug": "Builds a release version of the Cosmos SDK chain.",
			"cosmos:buildRelease": "Builds a release version of the Cosmos SDK chain.",
			"cosmos:install": "Installs a release version of the Cosmos SDK chain.",
			"cosmos:test": "Runs all main tests.",
			"cosmos:testIntegration": "Runs all integration for the Cosmos SDK chain.",
			"cosmos:testUnit": "Runs all unit tests for the Cosmos SDK chain.",
			"docs": "Starts a local docs page.",
			"format": "Run all formatters.",
			"generate": "Runs `go generate` on the entire project.",
			"generateCheck": "Runs `go generate` on the entire project and verifies that no files were changed.",
			"golangCiLint": "Run `golangci-lint`.",
			"golangCiLintFix": "Run `golangci-lint` with --fix.",
			"golines": "Run `golines`.",
			"gosec": "Run `gosec`.",
			"lint": "",
			"proto:all": "runs all proto related targets.",
			"proto:format": ".proto files.",
			"proto:gen": "generates protobuf source files.",
			"proto:genCheck": "checks that the generated protobuf source files are up-to-date.",
			"proto:lint": ".proto files.",
			"start": "starts a local development net and builds it if necessary.",
			"sync": "Runs 'go work sync' on the entire project.",
			"testIntegration": "Runs the integration tests.",
			"testIntegrationCover": "Runs the integration tests with coverage.",
			"testUnit": "Runs the unit tests.",
			"testUnitBenchmark": "Runs the unit tests with benchmarking.",
			"testUnitCover": "Runs the unit tests with coverage.",
			"testUnitRace": "Runs the unit tests with race detection.",
			"tidy": "Runs 'go tidy' on the entire project.",
		}

		keys := make([]string, 0, len(targets))
		for name := range targets {
			keys = append(keys, name)
		}
		_sort.Strings(keys)

		_fmt.Println("Targets:")
		w := _tabwriter.NewWriter(os.Stdout, 0, 4, 4, ' ', 0)
		for _, name := range keys {
			_fmt.Fprintf(w, "  %v\t%v\n", printName(name), targets[name])
		}
		err := w.Flush()
		return err
	}

	var ctx context.Context
	ctxCancel := func(){}

	// by deferring in a closure, we let the cancel function get replaced
	// by the getContext function.
	defer func() {
		ctxCancel()
	}()

	getContext := func() (context.Context, func()) {
		if ctx == nil {
			if args.Timeout != 0 {
				ctx, ctxCancel = context.WithTimeout(context.Background(), args.Timeout)
			} else {
				ctx, ctxCancel = context.WithCancel(context.Background())
			}
		}

		return ctx, ctxCancel
	}

	runTarget := func(logger *_log.Logger, fn func(context.Context) error) interface{} {
		var err interface{}
		ctx, cancel := getContext()
		d := make(chan interface{})
		go func() {
			defer func() {
				err := recover()
				d <- err
			}()
			err := fn(ctx)
			d <- err
		}()
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT)
		select {
		case <-sigCh:
			logger.Println("cancelling mage targets, waiting up to 5 seconds for cleanup...")
			cancel()
			cleanupCh := time.After(5 * time.Second)

			select {
			// target exited by itself
			case err = <-d:
				return err
			// cleanup timeout exceeded
			case <-cleanupCh:
				return _fmt.Errorf("cleanup timeout exceeded")
			// second SIGINT received
			case <-sigCh:
				logger.Println("exiting mage")
				return _fmt.Errorf("exit forced")
			}
		case <-ctx.Done():
			cancel()
			e := ctx.Err()
			_fmt.Printf("ctx err: %v\n", e)
			return e
		case err = <-d:
			// we intentionally don't cancel the context here, because
			// the next target will need to run with the same context.
			return err
		}
	}
	// This is necessary in case there aren't any targets, to avoid an unused
	// variable error.
	_ = runTarget

	handleError := func(logger *_log.Logger, err interface{}) {
		if err != nil {
			logger.Printf("Error: %+v\n", err)
			type code interface {
				ExitStatus() int
			}
			if c, ok := err.(code); ok {
				os.Exit(c.ExitStatus())
			}
			os.Exit(1)
		}
	}
	_ = handleError

	// Set MAGEFILE_VERBOSE so mg.Verbose() reflects the flag value.
	if args.Verbose {
		os.Setenv("MAGEFILE_VERBOSE", "1")
	} else {
		os.Setenv("MAGEFILE_VERBOSE", "0")
	}

	_log.SetFlags(0)
	if !args.Verbose {
		_log.SetOutput(_ioutil.Discard)
	}
	logger := _log.New(os.Stderr, "", 0)
	if args.List {
		if err := list(); err != nil {
			_log.Println(err)
			os.Exit(1)
		}
		return
	}

	if args.Help {
		if len(args.Args) < 1 {
			logger.Println("no target specified")
			os.Exit(2)
		}
		switch _strings.ToLower(args.Args[0]) {
			case "all":
				_fmt.Println("Runs a series of commonly used commands.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage all\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "build":
				_fmt.Println("Build builds the project.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage build\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "clean":
				_fmt.Println("Cleans the project.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage clean\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "contracts:build":
				_fmt.Println("Runs `forge build` in all smart contract directories.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage contracts:build\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "contracts:buildcheck":
				_fmt.Println("Check that the generated forge build source files are up to date.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage contracts:buildcheck\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "contracts:clean":
				_fmt.Println("Run `forge clean` in all smart contract directories.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage contracts:clean\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "contracts:fmt":
				_fmt.Println("Run `forge fmt` in all smart contract directories.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage contracts:fmt\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "contracts:test":
				_fmt.Println("Run `forge test` in all smart contract directories.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage contracts:test\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "contracts:testintegration":
				
				_fmt.Print("Usage:\n\n\tmage contracts:testintegration\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "contracts:testunit":
				_fmt.Println("Run `forge test` in all smart contract directories.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage contracts:testunit\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "cosmos:build":
				_fmt.Println("Build builds the Cosmos SDK chain.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage cosmos:build\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "cosmos:builddocker":
				_fmt.Println("Builds a release version of the Cosmos SDK chain.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage cosmos:builddocker\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "cosmos:builddockerdebug":
				_fmt.Println("Builds a release version of the Cosmos SDK chain.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage cosmos:builddockerdebug\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "cosmos:buildrelease":
				_fmt.Println("Builds a release version of the Cosmos SDK chain.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage cosmos:buildrelease\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "cosmos:install":
				_fmt.Println("Installs a release version of the Cosmos SDK chain.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage cosmos:install\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "cosmos:test":
				_fmt.Println("Runs all main tests.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage cosmos:test\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "cosmos:testintegration":
				_fmt.Println("Runs all integration for the Cosmos SDK chain.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage cosmos:testintegration\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "cosmos:testunit":
				_fmt.Println("Runs all unit tests for the Cosmos SDK chain.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage cosmos:testunit\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "docs":
				_fmt.Println("Starts a local docs page.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage docs\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "format":
				_fmt.Println("Run all formatters.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage format\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "generate":
				_fmt.Println("Runs `go generate` on the entire project.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage generate\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "generatecheck":
				_fmt.Println("Runs `go generate` on the entire project and verifies that no files were changed.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage generatecheck\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "golangcilint":
				_fmt.Println("Run `golangci-lint`.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage golangcilint\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "golangcilintfix":
				_fmt.Println("Run `golangci-lint` with --fix.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage golangcilintfix\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "golines":
				_fmt.Println("Run `golines`.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage golines\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "gosec":
				_fmt.Println("Run `gosec`.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage gosec\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "lint":
				
				_fmt.Print("Usage:\n\n\tmage lint\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "proto:all":
				_fmt.Println("All runs all proto related targets.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage proto:all\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "proto:format":
				_fmt.Println("Format .proto files.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage proto:format\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "proto:gen":
				_fmt.Println("Gen generates protobuf source files.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage proto:gen\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "proto:gencheck":
				_fmt.Println("GenCheck checks that the generated protobuf source files are up-to-date.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage proto:gencheck\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "proto:lint":
				_fmt.Println("Lint .proto files.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage proto:lint\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "start":
				_fmt.Println("Start starts a local development net and builds it if necessary. TODO: fix this?")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage start\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "sync":
				_fmt.Println("Runs 'go work sync' on the entire project.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage sync\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "testintegration":
				_fmt.Println("Runs the integration tests.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage testintegration\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "testintegrationcover":
				_fmt.Println("Runs the integration tests with coverage.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage testintegrationcover\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "testunit":
				_fmt.Println("Runs the unit tests.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage testunit\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "testunitbenchmark":
				_fmt.Println("Runs the unit tests with benchmarking.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage testunitbenchmark\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "testunitcover":
				_fmt.Println("Runs the unit tests with coverage.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage testunitcover\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "testunitrace":
				_fmt.Println("Runs the unit tests with race detection.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage testunitrace\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "tidy":
				_fmt.Println("Runs 'go tidy' on the entire project.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage tidy\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			default:
				logger.Printf("Unknown target: %q\n", args.Args[0])
				os.Exit(2)
		}
	}
	if len(args.Args) < 1 {
		if err := list(); err != nil {
			logger.Println("Error:", err)
			os.Exit(1)
		}
		return
	}
	for x := 0; x < len(args.Args); {
		target := args.Args[x]
		x++

		// resolve aliases
		switch _strings.ToLower(target) {
		
		}

		switch _strings.ToLower(target) {
		
			case "all":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"All\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "All")
				}
				
				wrapFn := func(ctx context.Context) error {
					All()
					return nil
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "build":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Build\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Build")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Build()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "clean":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Clean\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Clean")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Clean()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "contracts:build":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Contracts:Build\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Contracts:Build")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Contracts{}.Build()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "contracts:buildcheck":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Contracts:BuildCheck\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Contracts:BuildCheck")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Contracts{}.BuildCheck()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "contracts:clean":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Contracts:Clean\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Contracts:Clean")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Contracts{}.Clean()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "contracts:fmt":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Contracts:Fmt\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Contracts:Fmt")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Contracts{}.Fmt()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "contracts:test":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Contracts:Test\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Contracts:Test")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Contracts{}.Test()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "contracts:testintegration":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Contracts:TestIntegration\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Contracts:TestIntegration")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Contracts{}.TestIntegration()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "contracts:testunit":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Contracts:TestUnit\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Contracts:TestUnit")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Contracts{}.TestUnit()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "cosmos:build":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Cosmos:Build\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Cosmos:Build")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Cosmos{}.Build()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "cosmos:builddocker":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Cosmos:BuildDocker\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Cosmos:BuildDocker")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Cosmos{}.BuildDocker()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "cosmos:builddockerdebug":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Cosmos:BuildDockerDebug\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Cosmos:BuildDockerDebug")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Cosmos{}.BuildDockerDebug()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "cosmos:buildrelease":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Cosmos:BuildRelease\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Cosmos:BuildRelease")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Cosmos{}.BuildRelease()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "cosmos:install":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Cosmos:Install\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Cosmos:Install")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Cosmos{}.Install()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "cosmos:test":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Cosmos:Test\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Cosmos:Test")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Cosmos{}.Test()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "cosmos:testintegration":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Cosmos:TestIntegration\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Cosmos:TestIntegration")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Cosmos{}.TestIntegration()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "cosmos:testunit":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Cosmos:TestUnit\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Cosmos:TestUnit")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Cosmos{}.TestUnit()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "docs":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Docs\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Docs")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Docs()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "format":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Format\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Format")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Format()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "generate":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Generate\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Generate")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Generate()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "generatecheck":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"GenerateCheck\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "GenerateCheck")
				}
				
				wrapFn := func(ctx context.Context) error {
					return GenerateCheck()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "golangcilint":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"GolangCiLint\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "GolangCiLint")
				}
				
				wrapFn := func(ctx context.Context) error {
					return GolangCiLint()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "golangcilintfix":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"GolangCiLintFix\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "GolangCiLintFix")
				}
				
				wrapFn := func(ctx context.Context) error {
					return GolangCiLintFix()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "golines":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Golines\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Golines")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Golines()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "gosec":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Gosec\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Gosec")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Gosec()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "lint":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Lint\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Lint")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Lint()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "proto:all":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Proto:All\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Proto:All")
				}
				
				wrapFn := func(ctx context.Context) error {
					Proto{}.All()
					return nil
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "proto:format":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Proto:Format\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Proto:Format")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Proto{}.Format()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "proto:gen":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Proto:Gen\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Proto:Gen")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Proto{}.Gen()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "proto:gencheck":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Proto:GenCheck\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Proto:GenCheck")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Proto{}.GenCheck()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "proto:lint":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Proto:Lint\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Proto:Lint")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Proto{}.Lint()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "start":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Start\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Start")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Start()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "sync":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Sync\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Sync")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Sync()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "testintegration":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"TestIntegration\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "TestIntegration")
				}
				
				wrapFn := func(ctx context.Context) error {
					return TestIntegration()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "testintegrationcover":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"TestIntegrationCover\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "TestIntegrationCover")
				}
				
				wrapFn := func(ctx context.Context) error {
					return TestIntegrationCover()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "testunit":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"TestUnit\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "TestUnit")
				}
				
				wrapFn := func(ctx context.Context) error {
					return TestUnit()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "testunitbenchmark":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"TestUnitBenchmark\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "TestUnitBenchmark")
				}
				
				wrapFn := func(ctx context.Context) error {
					return TestUnitBenchmark()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "testunitcover":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"TestUnitCover\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "TestUnitCover")
				}
				
				wrapFn := func(ctx context.Context) error {
					return TestUnitCover()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "testunitrace":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"TestUnitRace\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "TestUnitRace")
				}
				
				wrapFn := func(ctx context.Context) error {
					return TestUnitRace()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "tidy":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Tidy\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Tidy")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Tidy()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
		
		default:
			logger.Printf("Unknown target specified: %q\n", target)
			os.Exit(2)
		}
	}
}





package cli

import "errors"

type Request struct {
	Action string
	JSON   bool
}

var ErrInvalidArgs = errors.New("invalid arguments")

func Parse(args []string) (Request, error) {
	if len(args) == 1 && args[0] == "version" {
		return Request{Action: "version"}, nil
	}
	if len(args) == 2 && args[0] == "version" && args[1] == "--json" {
		return Request{Action: "version", JSON: true}, nil
	}
	return Request{}, ErrInvalidArgs
}

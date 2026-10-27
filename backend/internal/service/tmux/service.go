package tmux

import "regexp"

var validName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)

type Session struct {
	Name     string `json:"name"`
	Created  int64  `json:"created"`
	Attached bool   `json:"attached"`
	Windows  int    `json:"windows"`
	Cwd      string `json:"cwd"`
}

type SessionClient interface {
	List() []Session
	Create(name string) error
	Kill(name string) error
	Has(name string) bool
	Cwd(session string) (string, error)
	SendText(session, text string, pressEnter bool) error
}

func ValidName(name string) bool {
	return validName.MatchString(name)
}

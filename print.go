package main

import "fmt"

const escape = "\x1b"

const (
	NONE = iota
	RED
	GREEN
	YELLOW
	BLUE
	PURPLE
)

func color(c int) string {
	if c == NONE {
		return fmt.Sprintf("%s[%dm", escape, c)
	}

	return fmt.Sprintf("%s[3%dm", escape, c)
}

func Format(c int, text string) string {
	return color(c) + text + color(NONE)
}

func (bs BeanState) String() string {
	return []string{
		"Unground", "Ground", "Brewed",
	}[bs]
}

func (bt BeanType) String() string {
	return []string{
		"Arabica", "Robusta", "Excelsa", "Liberica",
	}[bt]
}

func (s CoffeeSize) String() string {
	return []string{
		"Small", "Medium", "Large",
	}[s]
}

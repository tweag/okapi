package okapi

import (
	"fmt"
	"log"
	"regexp"
)

type SexpError struct{ msg string }

func (e SexpError) Error() string { return e.msg }

type SexpNode interface {
	List() ([]SexpNode, error)
	String() (string, error)
}

type SexpMap struct {
	Name   string
	Values map[string]SexpNode
}

func (m SexpMap) List() ([]SexpNode, error) {
	return nil, SexpError{fmt.Sprintf("SexpMap %#v cannot be converted to list", m)}
}

func (m SexpMap) String() (string, error) {
	return "", SexpError{fmt.Sprintf("SexpMap %#v cannot be converted to string", m)}
}

type SexpList struct{ Sub []SexpNode }

func (l SexpList) List() ([]SexpNode, error) { return l.Sub, nil }

func (l SexpList) String() (string, error) {
	if len(l.Sub) == 1 {
		return l.Sub[0].String()
	} else {
		return "", SexpError{fmt.Sprintf("SexpList %#v has multiple values", l)}
	}
}

type SexpString struct{ Content string }

func (s SexpString) List() ([]SexpNode, error) {
	return []SexpNode{s}, nil
}

func (s SexpString) String() (string, error) {
	return s.Content, nil
}

type SexpEmpty struct{}

func (s SexpEmpty) List() ([]SexpNode, error) { return nil, nil }
func (s SexpEmpty) String() (string, error) {
	return "", SexpError{"SexpEmpty cannot be converted to string"}
}

func consSexpList(nodes ...SexpNode) SexpNode { return SexpList{nodes} }

func sexpStrings(node SexpNode) ([]string, error) {
	var result []string
	l, err := node.List()
	if err != nil {
		return nil, err
	}
	if len(l) == 1 {
		if singleton, isSingleton := l[0].(SexpList); isSingleton {
			l = singleton.Sub
		}
	}
	for _, el := range l {
		s, err := el.String()
		if err != nil {
			return nil, SexpError{fmt.Sprintf("Element in SexpList %#v is not a string: %#v", l, el)}
		}
		result = append(result, s)
	}
	return result, nil
}

func sexpMap(elements []SexpNode) SexpNode {
	if len(elements) > 2 {
		canMap := false
		smap := make(map[string]SexpNode)
		name, nameIsString := elements[0].(SexpString)
		if nameIsString {
			canMap = true
			for _, node := range elements[1:] {
				l, isList := node.(SexpList)
				if isList && len(l.Sub) >= 1 {
					s, isString := l.Sub[0].(SexpString)
					if isString && smap[s.Content] == nil {
						var value SexpNode
						if len(l.Sub) == 2 {
							s, sErr := l.Sub[1].String()
							if sErr == nil {
								value = SexpString{s}
							} else {
								value = SexpList{l.Sub[1:]}
							}
						} else if len(l.Sub) == 1 {
							value = SexpEmpty{}
						} else {
							value = SexpList{l.Sub[1:]}
						}
						smap[s.Content] = value
					} else {
						canMap = false
					}
				} else {
					canMap = false
				}
			}
		}
		if canMap {
			return SexpMap{name.Content, smap}
		}
	}
	return SexpList{elements}
}

func sexp(tokens []string) (SexpNode, []string) {
	if len(tokens) > 0 {
		head := tokens[0]
		tail := tokens[1:]
		if head == "(" {
			sub, rest := sexpList(tail)
			return SexpList{sub}, rest
		} else if head == ")" {
			return SexpEmpty{}, tail
		} else {
			return SexpString{head}, tail
		}
	} else {
		return SexpEmpty{}, []string{}
	}
}

func sexpList(tokens []string) ([]SexpNode, []string) {
	done := false
	cur := tokens
	var result []SexpNode
	for !done {
		if len(cur) > 0 {
			if cur[0] == ")" {
				done = true
				cur = cur[1:]
			} else {
				next, rest := sexp(cur)
				result = append(result, next)
				cur = rest
			}
		} else {
			done = true
		}
	}
	return result, cur
}

func parseSexp(code string) []SexpNode {
	var tokens []string
	rex := regexp.MustCompile(`\(|\)|\s+|[^()\s]+`)
	ws := regexp.MustCompile(`^\s+$`)
	comment := regexp.MustCompile(`;.*\n`)
	withoutComments := comment.ReplaceAllString(code, "")
	for _, match := range rex.FindAllString(withoutComments, -1) {
		if !ws.MatchString(match) {
			tokens = append(tokens, match)
		}
	}
	items, rest := sexpList(tokens)
	if len(rest) > 0 {
		log.Fatalf("leftover tokens after parsing sexps: %#v", rest)
	}
	return items
}

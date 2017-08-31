package main

import (
	"bufio"
	"errors"
	"github.com/nsf/termbox-go"
	"github.com/tigrawap/slit/runes"
	"os"
	"regexp"
	"strings"
	"unicode"
)

type filterResult uint8

const filterLineMinLength int = 2

const (
	filterNoaction filterResult = iota
	filterIncluded
	filterExcluded
)

type filter interface {
	takeAction(str []rune, currentAction filterResult) filterResult
}

type SearchType struct {
	id    uint8 // id will be generated by order defined in init
	color termbox.Attribute
	name  string
}

var CaseSensitive = SearchType{
	color: termbox.ColorYellow,
	name:  "CaseS",
}

var RegEx = SearchType{
	color: termbox.ColorRed,
	name:  "RegEx",
}
var SearchTypeMap map[uint8]SearchType

type FilterAction uint8

const (
	FilterIntersect FilterAction = iota
	FilterUnion
	FilterExclude
)

const (
	FilterIntersectChar rune = '&'
	FilterUnionChar     rune = '+'
	FilterExcludeChar   rune = '-'
)

var FilterActionMap = map[rune]FilterAction{
	FilterIntersectChar: FilterIntersect,
	FilterUnionChar:     FilterUnion,
	FilterExcludeChar:   FilterExclude,
}

func init() {
	SearchTypeMap = make(map[uint8]SearchType)
	// Should maintain order, otherwise history will be corrupted.
	for i, r := range []*SearchType{&CaseSensitive, &RegEx} {
		r.id = uint8(i)
		SearchTypeMap[r.id] = *r
	}

}

// Follows regex return value pattern. nil if not found, slice of range if found
// Filter does not really need it, but highlighting also must search and requires it
type SearchFunc func(sub []rune) []int
type ActionFunc func(str []rune, currentAction filterResult) filterResult

type Filter struct {
	sub        []rune
	st         SearchType
	action     FilterAction
	takeAction ActionFunc
}

var BadFilterDefinition = errors.New("Bad filter definition")

func NewFilter(sub []rune, action FilterAction, searchType SearchType) (*Filter, error) {
	ff, err := getSearchFunc(searchType, sub)
	if err != nil {
		return nil, err
	}
	var af ActionFunc
	switch action {
	case FilterIntersect:
		af = buildIntersectionFunc(ff)
	case FilterUnion:
		af = buildUnionFunc(ff)
	case FilterExclude:
		af = buildExcludeFunc(ff)
	default:
		return nil, BadFilterDefinition
	}

	return &Filter{
		sub:        sub,
		st:         searchType,
		takeAction: af,
	}, nil
}

func getSearchFunc(searchType SearchType, sub []rune) (SearchFunc, error) {
	var ff SearchFunc
	switch searchType {
	case CaseSensitive:
		subLen := len(sub)
		ff = func(str []rune) []int {
			i := runes.Index(str, sub)
			if i == -1 {
				return nil
			}
			return []int{i, i + subLen}
		}
	case RegEx:
		re, err := regexp.Compile(string(sub))
		if err != nil {
			return nil, BadFilterDefinition
		}
		ff = func(str []rune) []int {
			return re.FindStringIndex(string(str))
		}
	default:
		return nil, BadFilterDefinition
	}
	return ff, nil
}

func IndexAll(searchFunc SearchFunc, runestack []rune) (indices [][]int) {
	if len(runestack) == 0 {
		return
	}
	var i int
	var ret []int
	f := 0
	indices = make([][]int, 0, 1)
	for {
		ret = searchFunc(runestack[i:])
		f++
		if ret == nil {
			break
		} else {
			ret[0] = ret[0] + i
			ret[1] = ret[1] + i
			indices = append(indices, ret)
			i = i + ret[1]
		}
		if i >= len(runestack) {
			break
		}
	}
	return
}

func buildUnionFunc(searchFunc SearchFunc) ActionFunc {
	return func(str []rune, currentAction filterResult) filterResult {
		if currentAction == filterIncluded {
			return filterIncluded
		}
		if searchFunc(str) != nil {
			return filterIncluded
		}
		return filterExcluded
	}

}

func buildIntersectionFunc(searchFunc SearchFunc) ActionFunc {
	return func(str []rune, currentAction filterResult) filterResult {
		if currentAction == filterExcluded {
			return filterExcluded
		}
		if searchFunc(str) != nil {
			return filterIncluded
		}
		return filterExcluded
	}
}

func buildExcludeFunc(searchFunc SearchFunc) ActionFunc {
	return func(str []rune, currentAction filterResult) filterResult {
		if currentAction == filterExcluded {
			return filterExcluded
		}
		if searchFunc(str) != nil {
			return filterExcluded
		}
		return filterIncluded
	}
}

func parseFilterLine(line string) (*Filter, error) {
	filteredLine := []rune(strings.TrimLeftFunc(line, unicode.IsSpace))
	if len(filteredLine) == 0 {
		return nil, nil
	}
	if len(filteredLine) < filterLineMinLength {
		return nil, errors.New("Filter is too short: " + string(filteredLine))
	}
	filterSign := filteredLine[0]
	action, ok := FilterActionMap[filterSign]
	if !ok {
		return nil, errors.New("Unknown filter type \"" + string(filterSign) + "\"")
	}
	filter, err := NewFilter(
		filteredLine[1:],
		action,
		CaseSensitive,
	)
	if err != nil {
		return nil, err
	}
	return filter, nil
}

func parseFiltersFile(filename string) ([]*Filter, error) {
	if err := validateRegularFile(filename); err != nil {
		return nil, err
	}
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)

	var filters []*Filter
	for scanner.Scan() {
		filter, err := parseFilterLine(scanner.Text())
		if err != nil {
			return nil, err
		}
		if filter == nil {
			continue
		}
		filters = append(filters, filter)
	}
	return filters, nil
}

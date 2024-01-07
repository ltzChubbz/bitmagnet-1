package model

import (
	"database/sql/driver"
	_ "embed"
	"encoding/json"
	"errors"
	"github.com/bitmagnet-io/bitmagnet/internal/regex"
	"github.com/facette/natsort"
	"regexp"
	"sort"
	"strings"
)

type Language string

type languageInfo struct {
	name          string
	alpha3        [3]rune
	lowerName     string
	aliases       []string
	lowerAliasMap map[string]struct{}
}

var languagesMap map[Language]languageInfo

func (l Language) info() languageInfo {
	return languagesMap[l]
}

func (l Language) String() string {
	return l.Alpha2()
}

func (l Language) Id() string {
	return l.Alpha2()
}

func (l Language) Alpha2() string {
	return string(l)
}

func (l Language) Alpha3() string {
	runes := l.info().alpha3
	return string(runes[0]) + string(runes[1]) + string(runes[2])
}

func (l Language) Name() string {
	return l.info().name
}

func (l Language) Aliases() []string {
	return l.info().aliases
}

func (l Language) IsValid() bool {
	_, ok := languagesMap[l]
	return ok
}

var languageNames []string

func LanguageNames() []string {
	return languageNames
}

var languageValues []Language

func LanguageValues() []Language {
	return languageValues
}

func LanguageValueStrings() []string {
	values := make([]string, len(languageValues))
	for i, lang := range languageValues {
		values[i] = lang.String()
	}
	return values
}

func newLanguagesRegex() *regexp.Regexp {
	var tokens []string
	for _, lang := range LanguageValues() {
		tokens = append(tokens, lang.Alpha2()+"dub")
		tokens = append(tokens, lang.Alpha3())
		tokens = append(tokens, lang.Name())
		tokens = append(tokens, lang.Aliases()...)
	}
	return regex.NewRegexFromNames(tokens...)
}

var languagesRegex *regexp.Regexp

type Languages map[Language]struct{}

func (l *Languages) Scan(value interface{}) error {
	if value == nil {
		*l = make(Languages)
		return nil
	}
	switch v := value.(type) {
	case []string:
		languages := make(Languages, len(v))
		for _, lang := range v {
			if lang := ParseLanguage(lang); lang.Valid {
				languages[lang.Language] = struct{}{}
			} else {
				return errors.New("invalid language")
			}
		}
		*l = languages
		return nil
	}
	return errors.New("invalid type for Languages")
}

func (l Languages) Value() (driver.Value, error) {
	values := make([]string, 0, len(l))
	if l != nil {
		for _, lang := range l.Slice() {
			values = append(values, lang.String())
		}
	}
	return values, nil
}

func (l *Languages) UnmarshalJSON(data []byte) error {
	var values []string
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	languages := make(Languages, len(values))
	for _, lang := range values {
		if lang := ParseLanguage(lang); lang.Valid {
			languages[lang.Language] = struct{}{}
		} else {
			return errors.New("invalid language")
		}
	}
	*l = languages
	return nil
}

func (l Languages) MarshalJSON() ([]byte, error) {
	values := make([]string, 0, len(l))
	for _, lang := range l.Slice() {
		values = append(values, lang.String())
	}
	return json.Marshal(values)
}

func (l Languages) Slice() []Language {
	values := make([]Language, 0, len(l))
	for lang := range l {
		values = append(values, lang)
	}
	sort.Slice(values, func(i, j int) bool {
		return natsort.Compare(values[i].Name(), values[j].Name())
	})
	return values
}

func InferLanguages(input string) Languages {
	languages := make(Languages)
	for {
		match := languagesRegex.FindStringSubmatchIndex(input)
		if match == nil {
			break
		}
		substr := input[match[2]:match[3]]
		substr = strings.TrimSuffix(substr, "dub")
		if lang := ParseLanguage(substr); lang.Valid {
			languages[lang.Language] = struct{}{}
		}
		input = input[match[1]:]
	}
	return languages
}

func (l *Language) Scan(value interface{}) error {
	switch v := value.(type) {
	case string:
		// jsonb_array_elements returns quoted strings
		if len(v) == 4 && v[0] == '"' && v[3] == '"' {
			v = v[1:3]
		}
		lang := ParseLanguage(v)
		if !lang.Valid {
			return errors.New("invalid language")
		}
		*l = lang.Language
		return nil
	case []byte:
		return l.Scan(string(v))
	}
	return errors.New("invalid type for Language")
}

func (l Language) Value() (driver.Value, error) {
	return l.String(), nil
}

type NullLanguage struct {
	Language Language
	Valid    bool
}

func (l *NullLanguage) Scan(value interface{}) error {
	if value == nil {
		l.Language, l.Valid = "", false
		return nil
	}
	switch v := value.(type) {
	case string:
		if v == "" {
			l.Valid = false
			return nil
		}
		lang := ParseLanguage(v)
		if !lang.Valid {
			return errors.New("invalid language")
		}
		l.Language, l.Valid = lang.Language, true
		return nil
	}
	return errors.New("invalid type for NullLanguage")
}

func (l NullLanguage) Value() (driver.Value, error) {
	if !l.Valid {
		return nil, nil
	}
	return l.Language.String(), nil
}

func NewNullLanguage(l Language) NullLanguage {
	return NullLanguage{l, true}
}

func ParseLanguage(name string) NullLanguage {
	name = strings.ToLower(name)
	if len(name) == 2 {
		lang := Language(name)
		if lang.IsValid() {
			return NewNullLanguage(lang)
		}
	}
	for lang, info := range languagesMap {
		if name == lang.Alpha3() {
			return NewNullLanguage(lang)
		}
		if info.lowerName == name {
			return NewNullLanguage(lang)
		}
		if _, ok := info.lowerAliasMap[name]; ok {
			return NewNullLanguage(lang)
		}
	}
	return NullLanguage{}
}

//go:embed languages.csv
var languagesCsvString string

func init() {
	csvLines := strings.Split(languagesCsvString, "\n")[1:]
	languagesMap = make(map[Language]languageInfo, len(csvLines))
	for _, line := range csvLines {
		if len(line) == 0 {
			continue
		}
		parts := strings.Split(line, ",")
		alpha2 := Language(parts[0])
		alpha3 := [3]rune{rune(parts[1][0]), rune(parts[1][1]), rune(parts[1][2])}
		name := parts[2]
		aliases := make([]string, 0)
		lowerAliasMap := make(map[string]struct{}, len(aliases))
		for _, alias := range strings.Split(parts[3], "|") {
			alias = strings.TrimSpace(alias)
			if len(alias) > 0 {
				aliases = append(aliases, alias)
				lowerAliasMap[strings.ToLower(alias)] = struct{}{}
			}
		}
		languagesMap[alpha2] = languageInfo{
			name:          name,
			alpha3:        alpha3,
			lowerName:     strings.ToLower(name),
			aliases:       aliases,
			lowerAliasMap: lowerAliasMap,
		}
		languageNames = append(languageNames, name)
		languageValues = append(languageValues, alpha2)
	}
	languagesRegex = newLanguagesRegex()
}

package config

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	envparse "github.com/caarlos0/env/v11"
)

// optionalValues are the optional numeric variables as the env parser converts them. The parser
// checks the type of each value; the range checks below decide whether a value is accepted.
type optionalValues struct {
	TopPassages        int           `env:"DIALLAK_TOP_PASSAGES"`
	MinScore           float64       `env:"DIALLAK_MIN_SCORE"`
	QuestionMaxChars   int           `env:"DIALLAK_QUESTION_MAX_CHARS"`
	ExcerptChars       int           `env:"DIALLAK_EXCERPT_CHARS"`
	ReplyMaxChars      int           `env:"DIALLAK_REPLY_MAX_CHARS"`
	MaxConcurrentAsks  int           `env:"DIALLAK_MAX_CONCURRENT_ASKS"`
	AskTimeout         time.Duration `env:"DIALLAK_ASK_TIMEOUT"`
	ModelTimeout       time.Duration `env:"DIALLAK_MODEL_TIMEOUT"`
	TrailRetention     time.Duration `env:"DIALLAK_TRAIL_RETENTION"`
	EmbedTimeout       time.Duration `env:"DIALLAK_EMBED_TIMEOUT"`
	EmbedMinSimilarity float64       `env:"DIALLAK_EMBED_MIN_SIMILARITY"`
}

// optionals holds the converted values and the raw text of every variable that was set. A
// variable that is not set, or set to the empty string, keeps its built-in default.
type optionals struct {
	values optionalValues
	raw    map[string]string
}

// optionalKeys lists the variable names the struct tags declare.
func optionalKeys() ([]string, error) {
	params, err := envparse.GetFieldParamsWithOptions(&optionalValues{}, envparse.Options{Environment: map[string]string{}})
	if err != nil {
		return nil, fmt.Errorf("read optional variable names: %w", err)
	}
	keys := make([]string, 0, len(params))
	for _, p := range params {
		keys = append(keys, p.Key)
	}
	return keys, nil
}

// readOptional converts the optional numeric variables. It reads only the declared names through
// getenv, never the process environment, and fails on a value of the wrong type.
func readOptional(getenv func(string) string) (optionals, error) {
	keys, err := optionalKeys()
	if err != nil {
		return optionals{}, err
	}
	o := optionals{raw: make(map[string]string, len(keys))}
	for _, k := range keys {
		if v := getenv(k); v != "" {
			o.raw[k] = v
		}
	}
	// Environment is never nil here, so the parser does not fall back to os.Environ.
	if err := envparse.ParseWithOptions(&o.values, envparse.Options{Environment: o.raw}); err != nil {
		return optionals{}, o.describeParseError(err)
	}
	return o, nil
}

// describeParseError names the variable and its value; the parser itself names only the field.
func (o optionals) describeParseError(err error) error {
	var parseErr envparse.ParseError
	if !errors.As(err, &parseErr) {
		return fmt.Errorf("optional settings: %w", err)
	}
	field, ok := reflect.TypeFor[optionalValues]().FieldByName(parseErr.Name)
	if !ok {
		return fmt.Errorf("optional settings: %w", err)
	}
	key, _, _ := strings.Cut(field.Tag.Get("env"), ",")
	want := "an integer"
	switch field.Type {
	case reflect.TypeFor[time.Duration]():
		want = "a valid duration"
	case reflect.TypeFor[float64]():
		want = "a decimal number"
	}
	return fmt.Errorf("%s must be %s: %q: %w", key, want, o.raw[key], parseErr)
}

// intIn returns val when envVar is set and val lies in [minVal, maxVal], and 0 when it is not set.
func (o optionals) intIn(envVar string, val, minVal, maxVal int) (int, error) {
	raw, set := o.raw[envVar]
	if !set {
		return 0, nil
	}
	if val < minVal || val > maxVal {
		return 0, fmt.Errorf("%s (%s) outside accepted range [%d, %d]", envVar, raw, minVal, maxVal)
	}
	return val, nil
}

// floatIn returns val when envVar is set and val lies above minExclusive and at most maxInclusive.
func (o optionals) floatIn(envVar string, val, minExclusive, maxInclusive float64) (float64, error) {
	raw, set := o.raw[envVar]
	if !set {
		return 0, nil
	}
	if math.IsNaN(val) || math.IsInf(val, 0) || val <= minExclusive || val > maxInclusive {
		return 0, fmt.Errorf("%s (%s) outside accepted range: above %g and at most %g", envVar, raw, minExclusive, maxInclusive)
	}
	return val, nil
}

// durationIn returns val when envVar is set and val lies in [minVal, maxVal].
func (o optionals) durationIn(envVar string, val, minVal, maxVal time.Duration) (time.Duration, error) {
	raw, set := o.raw[envVar]
	if !set {
		return 0, nil
	}
	if val < minVal || val > maxVal {
		return 0, fmt.Errorf("%s (%s) outside accepted range [%v, %v]", envVar, raw, minVal, maxVal)
	}
	return val, nil
}

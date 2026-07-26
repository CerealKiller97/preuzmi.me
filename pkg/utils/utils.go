package utils

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var ErrAbsolutePath = errors.New("path must be absolute")

func EnsureFolderExists(path string) error {
	if !filepath.IsAbs(path) {
		return ErrAbsolutePath
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		if err := os.Mkdir(path, 0o755); err != nil {
			// Folder already exists
			if errors.Is(err, os.ErrExist) {
				// silent ignore
				return nil
			}

			return err
		}
	}

	return nil
}

func MonthlyFolder(folder string) error {
	t := time.Now()
	year := t.Year()
	month := t.Month()

	for i := int(month); i <= 12; i++ {
		s := fmt.Sprintf("%02d-%d", i, year)
		if err := os.MkdirAll(path.Join(folder, s), 0o755); err != nil {
			return err
		}
	}

	return nil
}

func FormatFolderPath() string {
	t := time.Now()
	year := t.Year()
	month := t.Month()

	return fmt.Sprintf("%02d-%d", month, year)
}

// PreviousMonthFolder returns the "MM-YYYY" folder for the month before now,
// rolling the year back across January. Some providers (MTS, EPS) publish the
// previous month's bill, so a run in July fetches June's receipt and it must be
// filed under June's folder.
//
// Month arithmetic is done directly to avoid time.AddDate's day-overflow (e.g.
// July 31 minus a month lands back in July).
func PreviousMonthFolder() string {
	t := time.Now()
	year, month := t.Year(), int(t.Month())

	month--
	if month < 1 {
		month = 12
		year--
	}

	return fmt.Sprintf("%02d-%d", month, year)
}

// DateTimeFormat Default datetime format
const DateTimeFormat = "2006-01-02 15:04:05"

// ConfigureDefaultLogger Set default options for Zerolog with log level and pretty print
// If pretty print is false, it will write to Standard Output
func ConfigureDefaultLogger(level string, prettyPrint bool, output ...io.Writer) {
	zerologLevel, err := zerolog.ParseLevel(level)
	if err != nil {
		panic("Failed to parse logging level: " + level)
	}

	zerolog.SetGlobalLevel(zerologLevel)
	zerolog.TimeFieldFormat = DateTimeFormat
	zerolog.DurationFieldUnit = time.Microsecond
	zerolog.TimestampFunc = func() time.Time {
		return time.Now()
	}

	var w io.Writer

	if prettyPrint {
		w = zerolog.NewConsoleWriter()
	} else {
		w = os.Stdout
	}

	if len(output) > 0 {
		w = zerolog.MultiLevelWriter(w, output[0])
	}

	log.Logger = log.Output(w)
}

// New Returns an instance of zerolog.Logger with configured log level and pretty print flag
// If pretty print is false, it will write to Standard Output
func New(level string, prettyPrint bool) zerolog.Logger {
	var logger zerolog.Logger

	zerologLevel, err := zerolog.ParseLevel(level)
	if err != nil {
		panic("Failed to parse logging level: " + level)
	}

	var w io.Writer

	if prettyPrint {
		w = zerolog.NewConsoleWriter()
	} else {
		w = os.Stdout
	}

	logger = zerolog.New(w).
		With().
		Timestamp().
		Logger().
		Level(zerologLevel)

	return logger
}

// RandomString Returns random string of given length
func RandomString(n int32) string {
	buffer := make([]byte, n)

	_, err := rand.Read(buffer)
	if err != nil {
		return ""
	}

	return base64.RawURLEncoding.EncodeToString(buffer)
}

func GetAbsolutePath(path string) (string, error) {
	var err error

	if !filepath.IsAbs(path) {
		path, err = filepath.Abs(path)
		if err != nil {
			return "", err
		}

		return path, nil
	}

	return path, err
}

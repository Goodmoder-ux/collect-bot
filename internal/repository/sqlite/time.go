package sqlite

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"
)

type sqliteTime struct {
	time.Time
	valid bool
}

func (t *sqliteTime) Scan(value any) error {
	switch value := value.(type) {
	case nil:
		t.Time = time.Time{}
		t.valid = false
		return nil
	case time.Time:
		t.Time = value
		t.valid = true
		return nil
	case string:
		parsed, err := parseSQLiteTime(value)
		if err != nil {
			return err
		}
		t.Time = parsed
		t.valid = true
		return nil
	case []byte:
		parsed, err := parseSQLiteTime(string(value))
		if err != nil {
			return err
		}
		t.Time = parsed
		t.valid = true
		return nil
	default:
		return fmt.Errorf("unsupported SQLite time value %T", value)
	}
}

func (t sqliteTime) Value() (driver.Value, error) {
	if !t.valid {
		return nil, nil
	}
	return t.Time, nil
}

func parseSQLiteTime(value string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	fields := strings.Fields(value)
	if len(fields) == 4 && fields[2] == fields[3] {
		if parsed, err := time.Parse("2006-01-02 15:04:05 -0700", strings.Join(fields[:3], " ")); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("parse SQLite time %q", value)
}

package postgres

import (
	"encoding/json"
)

var errCodes = map[string]string{
	"uniqueConstraint": "23505",
}

type ErrMessage struct {
	Code     string `json:"Code"`
	Severity string `json:"Severity"`
	Message  string `json:"Message"`
}

func (dialector Dialector) Translate(err error) error {
	parsedErr, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		return err
	}

	var errMsg ErrMessage
	unmarshalErr := json.Unmarshal(parsedErr, &errMsg)
	if unmarshalErr != nil {
		return err
	}

	if errMsg.Code == errCodes["uniqueConstraint"] {
		// TODO: will add gorm.ErrDuplicatedKey here after this one merged https://github.com/go-gorm/gorm/pull/6004
		return err
	}

	return err
}

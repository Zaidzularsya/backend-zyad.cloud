package handler

import (
	"encoding/json"
	"errors"
	"io"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

var uuidParamPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// validUUID guards path ids so a malformed one is answered 404 instead of
// reaching PostgreSQL as a failing uuid cast (500).
func validUUID(id string) bool { return uuidParamPattern.MatchString(id) }

// bindStrictJSON decodes a required JSON body, rejecting unknown fields and
// trailing data, then applies the struct's binding tags.
func bindStrictJSON(c *gin.Context, dst any) error {
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON object")
	}
	return binding.Validator.ValidateStruct(dst)
}

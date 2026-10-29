package mikrotik

import "errors"

var (
	ErrRouterNotFound  = errors.New("mikrotik router not found")
	ErrRouterNameTaken = errors.New("mikrotik router name already used in this organization")
)

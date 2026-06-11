package permission

// Package permission holds core RBAC building blocks.
//
// Subpackages keep permission concerns separated:
// domain contains RBAC entities, repository reads PostgreSQL permission tables,
// service exposes permission use cases, policy checks permission sets,
// middleware adapts permission checks to Gin, handler exposes optional HTTP
// endpoints, and seeder prepares initial role/permission data.

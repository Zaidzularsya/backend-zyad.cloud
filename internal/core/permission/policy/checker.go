package policy

func HasAll(userPermissions []string, requiredPermissions []string) bool {
	if len(requiredPermissions) == 0 {
		return true
	}

	allowed := make(map[string]struct{}, len(userPermissions))
	for _, permission := range userPermissions {
		allowed[permission] = struct{}{}
	}

	for _, permission := range requiredPermissions {
		if _, ok := allowed[permission]; !ok {
			return false
		}
	}

	return true
}

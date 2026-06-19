import glob, re

for path in glob.glob('internal/modules/landing/handler/admin_*_handler.go'):
    with open(path, 'r') as f:
        content = f.read()

    # fix tenant.GetScope
    content = content.replace("tenant.GetScope", "coretenant.RequireScope")
    
    # fix unused userID in navigation and others where auth is imported but not used
    # Wait, the previous error showed: "declared and not used: userID"
    # because the struct doesn't use it.
    
    # If the file has "userID, err := auth.GetUserFromContext" or "userID := permissionmiddleware.UserID(c)" but doesn't use userID, 
    # we should replace it with `_ = userID` or remove it.
    # It's easier to just replace `userID := permissionmiddleware.UserID(c)` with `_ = permissionmiddleware.UserID(c)` if `userID` isn't used anywhere else.
    
    # Or just replace all unused userIDs.
    # Actually, in navigation handler line 175 and 216, userID is declared:
    # user, err := auth.GetUserFromContext(c)
    # ...
    # Let's replace "user, err := auth.GetUserFromContext(c)" with "_, err := auth.GetUserFromContext(c)" if user is not used.
    # But since it's "user.ID.String()", in navigation handler, maybe I changed it to "userID := permissionmiddleware.UserID(c)".
    # No, I didn't change it successfully because the python script failed previously.
    
    # Let's just run goimports or go fmt? No, let's fix manually.
    pass

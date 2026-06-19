import os, re, glob

def process_file(filepath):
    with open(filepath, 'r') as f:
        content = f.read()

    # 1. Replace GetUserFromContext
    content = re.sub(
        r'user,\s*err\s*:=\s*auth\.GetUserFromContext\(c\)\n\s*if\s*err\s*!=\s*nil\s*\{[^}]+\}\n',
        r'userID := permissionmiddleware.UserID(c)\n',
        content,
        flags=re.MULTILINE|re.DOTALL
    )
    content = content.replace('user.ID.String()', 'userID')

    # 2. Replace corehttp.Success
    # Case with res list mapping
    content = re.sub(
        r'res\s*:=\s*make\(\[\]dto\.[A-Za-z0-9_]+Response,\s*len\(([a-zA-Z0-9_]+)\)\)\n\s*for\s*i,\s*[a-zA-Z0-9_]+\s*:=\s*range\s*\1\s*\{\n\s*res\[i\]\s*=\s*dto\.Map[A-Za-z0-9_]+Response\([a-zA-Z0-9_]+\)\n\s*\}\n\s*corehttp\.Success\(c,\s*res,\s*nil\)',
        r'corehttp.OK(c, "success", \1)',
        content,
        flags=re.MULTILINE|re.DOTALL
    )

    # Case with single item mapping
    content = re.sub(
        r'corehttp\.Success\(c,\s*dto\.Map[A-Za-z0-9_]+Response\(([a-zA-Z0-9_]+)\),\s*nil\)',
        r'corehttp.OK(c, "success", \1)',
        content
    )

    # General Success case with map map[string]string{"message": "deleted"} -> corehttp.OK(c, "deleted", nil)
    content = re.sub(
        r'corehttp\.Success\(c,\s*map\[string\]string\{"message":\s*"([^"]+)"\},\s*nil\)',
        r'corehttp.OK(c, "\1", nil)',
        content
    )
    
    content = re.sub(
        r'corehttp\.Success\(c,\s*([^,]+),\s*nil\)',
        r'corehttp.OK(c, "success", \1)',
        content
    )

    # 3. Remove "zyad.cloud/internal/core/auth" if present
    content = re.sub(r'\t"zyad\.cloud/internal/core/auth"\n', '', content)

    # 4. Remove dto if not used. If there's DTO Request structs, dto is used, so don't remove.

    with open(filepath, 'w') as f:
        f.write(content)

for filepath in glob.glob('internal/modules/landing/handler/admin_*_handler.go'):
    process_file(filepath)


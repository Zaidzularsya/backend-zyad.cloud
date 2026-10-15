package service

import (
	"path"
	"strings"
)

// MaxTotalAttachmentBytes stays under the 20MB request body limit of the
// nginx front (the rest is headers and the HTML body). Each file is also
// limited by the asset module (10MB).
const MaxTotalAttachmentBytes = 18 * 1024 * 1024

// attachmentTypes maps allowed extensions to the MIME type the file is
// stored and sent with. The browser-supplied type is ignored: it varies by
// OS (a .csv is often application/vnd.ms-excel on Windows).
var attachmentTypes = map[string]string{
	".pdf":  "application/pdf",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
	".gif":  "image/gif",
	".doc":  "application/msword",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xls":  "application/vnd.ms-excel",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".ppt":  "application/vnd.ms-powerpoint",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".csv":  "text/csv",
	".txt":  "text/plain",
	".zip":  "application/zip",
}

// attachmentMimeTypes is the asset-module allow-list for email attachments.
var attachmentMimeTypes = func() map[string]bool {
	allowed := map[string]bool{}
	for _, mimeType := range attachmentTypes {
		allowed[mimeType] = true
	}
	return allowed
}()

func attachmentMimeType(filename string) (string, bool) {
	mimeType, ok := attachmentTypes[strings.ToLower(path.Ext(filename))]
	return mimeType, ok
}

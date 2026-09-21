package archive

import "errors"

const MediaType = "application/vnd.nem.archive.v2"

var ErrNotFound = errors.New("archive not found")

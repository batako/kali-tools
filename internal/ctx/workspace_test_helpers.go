package ctx

import "strconv"

func workspaceIDString(id int64) string {
	return strconv.FormatInt(id, 10)
}

package api

import (
	"net/http"

	"github.com/Tk21111/whiteboard_server/db"
)

func PermHelper(result *db.ViewResult, w http.ResponseWriter) bool {
	switch *result {
	case db.NotExist:
		http.Error(w, "room not exist", http.StatusNotFound)
		return false
	case db.NoPerm:
		http.Error(w, "no perm", http.StatusForbidden)
		return false
	}
	return true
}

package db

import (
	"fmt"

	"github.com/Tk21111/whiteboard_server/config"
)

func EnsureUserInRoom(roomId, userId string) (config.Role, error) {

	roomRole, err := GetUserRoomRole(roomId, userId)
	if err != nil && roomRole == -2 {
		return -1, err
	}

	if roomRole >= 0 {
		return config.Role(roomRole), nil
	}

	globalRole, err := GetUserRole(userId)
	if err != nil {
		return -1, err
	}

	if globalRole == int(config.RoleModerator) || globalRole == int(config.RoleOwner) {

		exist, err := CheckRoomExisted(roomId)
		if err != nil {
			return -1, err
		}

		if !exist {
			if err := CreateRoom(roomId, userId, 1); err != nil {
				return -1, err
			}
		}

		if err := JoinRoom(roomId, userId, config.RoleOwner); err != nil {
			return -1, err
		}

		return config.RoleOwner, nil
	}

	view, err := CheckCanViewRoom(roomId, userId)
	if err != nil {
		return -1, err
	}

	if view != Perm {
		return -1, fmt.Errorf("forbidden")
	}

	if err := JoinRoom(roomId, userId, config.RoleMember); err != nil {
		return -1, err
	}

	return config.RoleModerator, nil
}

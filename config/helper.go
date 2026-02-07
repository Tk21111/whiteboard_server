package config

func IntToRole(roleInt int) Role {

	var role Role
	switch roleInt {
	case 0:
		role = RoleGuest
	case 1:
		role = RoleMember
	case 2:
		role = RoleModerator
	case 3:
		role = RoleOwner
	}

	return role

}

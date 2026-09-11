package model

// 功能权限（应用级）解析。
//
// 历史沿革：早期是多级权限 —— 全局开关(SystemApp) > 用户个人覆盖(User.AppPerms)
// > 用户组设置(UserGroup.AppPerms) > 默认允许。
//
// 2026-09 单用户私有化改造后，用户组与个人权限覆盖都已删除，系统只服务一个管理员账号，
// 因此除了"全局功能开关"以外不再有任何权限限制。

// AppAllowed 判断用户能否使用功能 key。
// 单用户私有部署下：全局开关关闭则不可用；开关打开即一律可用（唯一账号是管理员）。
//
// 保留 *User 形参是为了不改动大量调用点，当前不使用。
func AppAllowed(key string, _ *User) bool {
	return AppEnabled(key)
}

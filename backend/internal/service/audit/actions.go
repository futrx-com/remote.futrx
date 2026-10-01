package audit

// Action names are dot-separated and hierarchical so the read API can filter
// by prefix: "project." matches every project action, "project.secret."
// matches only the secret ones. Keep the segments stable — operators build
// queries and alerts against these strings.
const (
	ActionAuthLoginSuccess = "auth.login.success"
	ActionAuthLoginFailure = "auth.login.failure"
	ActionAuthLogout       = "auth.logout"
	ActionAuthAdminClaim   = "auth.admin.claim"

	ActionUserInvite     = "user.invite"
	ActionUserRemove     = "user.remove"
	ActionUserRoleChange = "user.role-change"

	ActionProjectCreate       = "project.create"
	ActionProjectRename       = "project.rename"
	ActionProjectDelete       = "project.delete"
	ActionProjectMemberAdd    = "project.member.add"
	ActionProjectMemberRemove = "project.member.remove"

	ActionProjectSecretRead   = "project.secret.read"
	ActionProjectSecretSet    = "project.secret.set"
	ActionProjectSecretDelete = "project.secret.delete"

	ActionProjectContainerStart   = "project.container.start"
	ActionProjectContainerStop    = "project.container.stop"
	ActionProjectContainerRestart = "project.container.restart"
	ActionProjectContainerRecycle = "project.container.recycle"
	ActionProjectContainerRepair  = "project.container.repair-network"
	ActionProjectContainerLimits  = "project.container.limits"

	ActionProjectBrowserStart = "project.browser.start"
	ActionProjectBrowserStop  = "project.browser.stop"

	ActionChatCreate = "chat.create"
	ActionChatDelete = "chat.delete"

	ActionAgentRunStart  = "agent.run.start"
	ActionAgentRunCancel = "agent.run.cancel"

	ActionScheduleCreate = "schedule.create"
	ActionScheduleUpdate = "schedule.update"
	ActionScheduleArm    = "schedule.arm"
	ActionScheduleDelete = "schedule.delete"
	ActionScheduleRunNow = "schedule.run-now"

	ActionSettingsGoogleOAuth     = "settings.google-oauth.configure"
	ActionSettingsAgentConnect    = "settings.agent.connect"
	ActionSettingsAgentDisconnect = "settings.agent.disconnect"

	ActionSelfUpdateTrigger = "self-update.trigger"

	ActionWorkspaceFileUpload      = "workspace.file.upload"
	ActionWorkspaceFileDownload    = "workspace.file.download"
	ActionWorkspaceArchiveDownload = "workspace.file.archive-download"
	ActionWorkspaceGitCheckout     = "workspace.git.checkout"
	ActionWorkspaceIDEOpen         = "workspace.ide.open"
	ActionWorkspaceTerminalOpen    = "workspace.terminal.open"
	ActionWorkspaceTerminalClose   = "workspace.terminal.close"
)

// Target types recorded alongside an action.
const (
	TargetProject  = "project"
	TargetUser     = "user"
	TargetSecret   = "secret"
	TargetChat     = "chat"
	TargetSchedule = "schedule"
	TargetAgent    = "agent"
	TargetFile     = "file"
	TargetSession  = "session"
	TargetServer   = "server"
)

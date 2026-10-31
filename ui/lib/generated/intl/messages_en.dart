// DO NOT EDIT. This is code generated via package:intl/generate_localized.dart
// This is a library that provides messages for a en locale. All the
// messages from the main program should be duplicated here with the same
// function name.

// Ignore issues from commonly used lints in this file.
// ignore_for_file:unnecessary_brace_in_string_interps, unnecessary_new
// ignore_for_file:prefer_single_quotes,comment_references, directives_ordering
// ignore_for_file:annotate_overrides,prefer_generic_function_type_aliases
// ignore_for_file:unused_import, file_names, avoid_escaping_inner_quotes
// ignore_for_file:unnecessary_string_interpolations, unnecessary_string_escapes

import 'package:intl/intl.dart';
import 'package:intl/message_lookup_by_library.dart';

final messages = new MessageLookup();

typedef String MessageIfAbsent(String messageStr, List<dynamic> args);

class MessageLookup extends MessageLookupByLibrary {
  String get localeName => 'en';

  static String m0(createdAt) => "Added ${createdAt}";

  static String m1(label) => "Delete ${label}?";

  static String m2(rName) => "Delete role ${rName}?";

  static String m3(rName) => "Only ${rName}";

  static String m4(pName) => "Only ${pName}";

  static String m5(tName) => "Revoke ${tName}?";

  static String m6(minMemoryMb) => "At least ${minMemoryMb}";

  static String m7(addonName) => "Delete ${addonName}?";

  static String m8(addonName) => "Link ${addonName} to an app";

  static String m9(appName) => "Linked. Redeploy ${appName} to apply.";

  static String m10(host, port) =>
      "Reachable only from apps in the same project, at ${host}:${port}. Link an app below to give it these settings as variables.";

  static String m11(vars) => "All apps get: ${vars}";

  static String m12(alertsLength) => "and ${alertsLength} more";

  static String m13(chName) => "Delete ${chName}?";

  static String m14(title) => "${title}: enter a number";

  static String m15(chName) => "Test message sent to ${chName}";

  static String m16(appName) =>
      "Build started: it deploys ${appName} when it is done";

  static String m17(list) => "Changed ${list} are saved but not live yet.";

  static String m18(host) => "Copied ${host}";

  static String m19(e) => "Could not discard: ${e}";

  static String m20(appName) => "Delete ${appName}?";

  static String m21(what) => "Delete ${what}?";

  static String m22(slug) => "Delete the preview ${slug}?";

  static String m23(vName) => "Delete the data of ${vName}?";

  static String m24(volume) => "Files · ${volume}";

  static String m25(eName) =>
      "${eName} is not a text file — download it instead";

  static String m26(appName) =>
      "${appName} is saved as \"not deployed\"; you can close the wizard and continue later from the app page.";

  static String m27(port, proto, nodePort) =>
      "Port ${port}/${proto} is open on the server as ${nodePort}";

  static String m28(days) => "${days} days";

  static String m29(parent, number) =>
      "Preview of ${parent} · pull request #${number}";

  static String m30(parent) =>
      "Rebuilt on new commits and deleted when the pull request closes. Change settings on ${parent}.";

  static String m31(t) => "Promote to ${t}…";

  static String m32(ready, replicas) => "${ready} / ${replicas} ready";

  static String m33(host) => "Remove ${host}?";

  static String m34(path) => "Remove ${path}?";

  static String m35(fName) => "Replace ${fName}?";

  static String m36(eName) => "Saved ${eName}";

  static String m37(index, stepsLength) => "Step ${index} of ${stepsLength}";

  static String m38(label) =>
      "This file suits ${label} better than the version you chose.";

  static String m39(slug) => "Type ${slug} to confirm:";

  static String m40(size) => "${size} uploaded and checked";

  static String m41(round) => "Uploading… ${round}%";

  static String m42(label) => "Use ${label}";

  static String m43(vName) => "Volume ${vName} deleted";

  static String m44(appName) =>
      "\"${appName}\" was created but not deployed yet. Keep it to finish later, or discard it.";

  static String m45(pName) => "Continue with ${pName}";

  static String m46(subject) => "Backup of ${subject} started";

  static String m47(what) => "${what} Changes since then are lost.";

  static String m48(tName) => "Checking ${tName}…";

  static String m49(nextRun) => "Next: ${nextRun}";

  static String m50(subject, targetName) =>
      "No more backups of ${subject} to ${targetName}. Existing snapshots stay in the repository.";

  static String m51(tName) => "Remove ${tName}?";

  static String m52(shortId) => "Restore ${shortId}?";

  static String m53(subject) => "Snapshots · ${subject}";

  static String m54(count) =>
      "${Intl.plural(count, zero: 'No apps', one: '1 app', other: '${count} apps')}";

  static String m55(baseDomain, baseHost) =>
      "The *.${baseDomain} record serves every app\'s default domain. A CNAME to ${baseHost} works for subdomains of your own domains once it exists.";

  static String m56(ip) => "Block ${ip}";

  static String m57(domain) => "Delete ${domain}?";

  static String m58(baseWildcard) => "Get ${baseWildcard}";

  static String m59(ms) => "${ms} ms";

  static String m60(pod) => "Before the last restart · ${pod}";

  static String m61(linesLength) => "Copied ${linesLength} lines";

  static String m62(visibleLength) => "Copied ${visibleLength} lines";

  static String m63(replicas) =>
      "Totals across ${replicas} replicas; the dashed line is what all of them requested together.";

  static String m64(sampleImage) => "Image: ${sampleImage} · 50m CPU · 64 MB";

  static String m65(what) => "Choose ${what}";

  static String m66(slug) =>
      "Copied — apply it with: nectar apply -f nectar.yaml --project ${slug}";

  static String m67(appName) => "Copy ${appName}";

  static String m68(gName) => "Delete the group ${gName}?";

  static String m69(slug) => "Environment of ${slug}";

  static String m70(what) => "Import ${what}";

  static String m71(key) =>
      "Invalid variable name \"${key}\": use letters, digits and _";

  static String m72(appName, target) => "Promote ${appName} to ${target}?";

  static String m73(slug, slug2, slug3) =>
      "The live build of ${slug} (its image or uploaded file) is deployed to ${slug2}/${slug3}. Its settings and variables there stay as they are.";

  static String m74(nodeName) => "Drain ${nodeName}?";

  static String m75(nodeName) => "Drain ${nodeName} anyway?";

  static String m76(historyName, range) =>
      "History · ${historyName} · last ${range}";

  static String m77(keepArtifacts, auditDays, alertDays) =>
      "Kept: the last ${keepArtifacts} uploads per app (and every upload a live release uses), ${auditDays} days of audit log, ${alertDays} days of resolved alerts. Cleanup runs every hour.";

  static String m78(nodeName) => "Make ${nodeName} an edge node?";

  static String m79(nodeName) => "Remove ${nodeName} from the cluster?";

  static String m80(nodeName) => "Stop serving traffic on ${nodeName}?";

  static String m81(requestedBytes) =>
      "${requestedBytes} · usage shows while the app or database runs";

  static String m82(ip, lastSeenAt, createdAt) =>
      "${ip} · active ${lastSeenAt} · since ${createdAt}";

  static String m83(ip) =>
      "Allow only these IP addresses or networks (one per line, e.g. 203.0.113.7 or 203.0.113.0/24). Empty = everyone. Your address: ${ip}. An SSH tunnel to the server always works, so a wrong list can be fixed.";

  static String m84(username, updatedAt) =>
      "${username} · changed ${updatedAt}";

  static String m85(left) =>
      "Only ${left} recovery codes left: create new ones.";

  static String m86(name) => "Remove the passkey ${name}?";

  static String m87(when) => "used ${when}";

  static String m88(pName) => "Remove ${pName}?";

  static String m89(host) => "Remove the login for ${host}?";

  static String m90(amount) =>
      "The server\'s clock is ${amount} ahead of this device: times in the panel look wrong, and two-factor codes and certificates can fail. On the server run: sudo timedatectl set-ntp true";

  static String m91(amount) =>
      "The server\'s clock is ${amount} behind this device: times in the panel look wrong, and two-factor codes and certificates can fail. On the server run: sudo timedatectl set-ntp true";

  static String m92(v) =>
      "k3s upgrade to ${v} started: node versions update here as it progresses";

  static String m93(summary) => "Roll back ${summary}?";

  static String m94(component) => "Update ${component}";

  static String m95(v) => "Update k3s to ${v}?";

  static String m96(summary) => "Updating · ${summary}";

  final messages = _notInlinedMessages(_notInlinedMessages);
  static Map<String, Function> _notInlinedMessages(_) => <String, Function>{
    "acceARoleIsASet": MessageLookupByLibrary.simpleMessage(
      "A role is a set of permissions. Users get roles everywhere or for one project; API tokens can be limited to a role.",
    ),
    "acceATokenNeverHasMore": MessageLookupByLibrary.simpleMessage(
      "A token never has more rights than you. For CI: the Deployer role and one project.",
    ),
    "acceAddMember": MessageLookupByLibrary.simpleMessage("Add member"),
    "acceAddRole": MessageLookupByLibrary.simpleMessage("Add role"),
    "acceAddUser": MessageLookupByLibrary.simpleMessage("Add user"),
    "acceAdded": m0,
    "acceAllNone": MessageLookupByLibrary.simpleMessage("all / none"),
    "acceAnyHttpClientWorksToo": MessageLookupByLibrary.simpleMessage(
      "Any HTTP client works too: Authorization: Bearer nat_… on the API (see api/openapi.yaml).",
    ),
    "acceApiTokens": MessageLookupByLibrary.simpleMessage("API tokens"),
    "acceBuiltInRolesCannotBe": MessageLookupByLibrary.simpleMessage(
      "Built-in roles cannot be changed. Clone it to make your own version.",
    ),
    "acceCannotSignInSessionsAnd": MessageLookupByLibrary.simpleMessage(
      "Cannot sign in; sessions and tokens stop working",
    ),
    "acceChangingRolesNeedsRoleManage": MessageLookupByLibrary.simpleMessage(
      "Changing roles needs role.manage.",
    ),
    "acceClone": MessageLookupByLibrary.simpleMessage("Clone"),
    "acceCopyYourToken": MessageLookupByLibrary.simpleMessage(
      "Copy your token",
    ),
    "acceCreateOneForGithubActions": MessageLookupByLibrary.simpleMessage(
      "Create one for GitHub Actions or the nectar CLI. A Deployer token for one project is enough to deploy.",
    ),
    "acceDelete": m1,
    "acceDeleteRole": m2,
    "acceDescription": MessageLookupByLibrary.simpleMessage("Description"),
    "acceDisabled": MessageLookupByLibrary.simpleMessage("Disabled"),
    "acceEveryoneSTokens": MessageLookupByLibrary.simpleMessage(
      "Everyone\'s tokens",
    ),
    "acceEverythingYouMayDo": MessageLookupByLibrary.simpleMessage(
      "Everything you may do",
    ),
    "acceEverywhere": MessageLookupByLibrary.simpleMessage("Everywhere"),
    "acceExpires": MessageLookupByLibrary.simpleMessage("Expires"),
    "acceForCiAndScriptsA": MessageLookupByLibrary.simpleMessage(
      "For CI and scripts. A token acts as you, limited to a role and a project when you choose them.",
    ),
    "acceGithubActionsShop": MessageLookupByLibrary.simpleMessage(
      "GitHub Actions: shop",
    ),
    "acceGlobalOnlyPermissionsHaveNo": MessageLookupByLibrary.simpleMessage(
      "\"Global only\" permissions have no effect when the role is given for one project.",
    ),
    "acceIn30Days": MessageLookupByLibrary.simpleMessage("in 30 days"),
    "acceIn90Days": MessageLookupByLibrary.simpleMessage("in 90 days"),
    "acceInAYear": MessageLookupByLibrary.simpleMessage("in a year"),
    "acceItIsShownOnlyNow": MessageLookupByLibrary.simpleMessage(
      "It is shown only now. Store it as a secret (e.g. NECTAR_TOKEN in GitHub).",
    ),
    "acceMembers": MessageLookupByLibrary.simpleMessage("Members"),
    "acceNewApiToken": MessageLookupByLibrary.simpleMessage("New API token"),
    "acceNewRole": MessageLookupByLibrary.simpleMessage("New role"),
    "acceNewToken": MessageLookupByLibrary.simpleMessage("New token"),
    "acceNewUsersHaveNoRoles": MessageLookupByLibrary.simpleMessage(
      "New users have no roles: give them one after creating them.",
    ),
    "acceNoTokens": MessageLookupByLibrary.simpleMessage("No tokens."),
    "acceNoTokensYet": MessageLookupByLibrary.simpleMessage("No tokens yet"),
    "acceOnly": m3,
    "acceOnly2": m4,
    "acceOwnersCanDoEverythingRoles": MessageLookupByLibrary.simpleMessage(
      "Owners can do everything; roles do not change that.",
    ),
    "accePassword10Characters": MessageLookupByLibrary.simpleMessage(
      "Password (10+ characters)",
    ),
    "acceRemoveFromProject": MessageLookupByLibrary.simpleMessage(
      "Remove from project",
    ),
    "acceReset2fa": MessageLookupByLibrary.simpleMessage("Reset 2FA"),
    "acceRevoke": m5,
    "acceRevoke2": MessageLookupByLibrary.simpleMessage("Revoke"),
    "acceRights": MessageLookupByLibrary.simpleMessage("Rights"),
    "acceRole": MessageLookupByLibrary.simpleMessage("Role"),
    "acceRoles": MessageLookupByLibrary.simpleMessage("Roles"),
    "acceRolesStillAssignedToUsers": MessageLookupByLibrary.simpleMessage(
      "Roles still assigned to users cannot be deleted.",
    ),
    "acceScriptsAndPipelinesThatUse": MessageLookupByLibrary.simpleMessage(
      "Scripts and pipelines that use it stop working immediately.",
    ),
    "acceSendAnInvitation": MessageLookupByLibrary.simpleMessage(
      "Send an invitation",
    ),
    "acceSendInvitationResetLink": MessageLookupByLibrary.simpleMessage(
      "Send invitation / reset link",
    ),
    "acceShareTheInvitationLink": MessageLookupByLibrary.simpleMessage(
      "Share the invitation link",
    ),
    "acceThePanelCannotSendEmails": MessageLookupByLibrary.simpleMessage(
      "The panel cannot send emails (Settings → Email). Send this link yourself; it works for 7 days.",
    ),
    "acceTheyCanNoLongerSign": MessageLookupByLibrary.simpleMessage(
      "They can no longer sign in; their API tokens stop working. The audit log keeps their actions.",
    ),
    "acceTheyChooseTheirOwnPassword": MessageLookupByLibrary.simpleMessage(
      "They choose their own password",
    ),
    "acceUser": MessageLookupByLibrary.simpleMessage("User"),
    "acceUserCreated": MessageLookupByLibrary.simpleMessage("User created"),
    "acceUsers": MessageLookupByLibrary.simpleMessage("Users"),
    "acceUsingAToken": MessageLookupByLibrary.simpleMessage("Using a token"),
    "acceWhere": MessageLookupByLibrary.simpleMessage("Where"),
    "acceWhoCanSignInAnd": MessageLookupByLibrary.simpleMessage(
      "Who can sign in, and what they may do. Roles apply everywhere or to one project.",
    ),
    "acceYouCanOnlyGiveOr": MessageLookupByLibrary.simpleMessage(
      "You can only give or take away roles whose permissions you hold yourself.",
    ),
    "acceYouCanOnlyGiveRoles": MessageLookupByLibrary.simpleMessage(
      "You can only give roles whose permissions you hold in this project.",
    ),
    "acceYourRoleDoesNotInclude": MessageLookupByLibrary.simpleMessage(
      "Your role does not include user.manage.",
    ),
    "acceYourTokens": MessageLookupByLibrary.simpleMessage("Your tokens"),
    "actionCancel": MessageLookupByLibrary.simpleMessage("Cancel"),
    "actionClose": MessageLookupByLibrary.simpleMessage("Close"),
    "actionDelete": MessageLookupByLibrary.simpleMessage("Delete"),
    "actionOk": MessageLookupByLibrary.simpleMessage("OK"),
    "actionRefresh": MessageLookupByLibrary.simpleMessage("Refresh"),
    "actionRetry": MessageLookupByLibrary.simpleMessage("Try again"),
    "actionSave": MessageLookupByLibrary.simpleMessage("Save"),
    "addonAddADatabase": MessageLookupByLibrary.simpleMessage("Add a database"),
    "addonAddDatabase": MessageLookupByLibrary.simpleMessage("Add database"),
    "addonApp": MessageLookupByLibrary.simpleMessage("App"),
    "addonAtLeast": m6,
    "addonCache": MessageLookupByLibrary.simpleMessage("CACHE_"),
    "addonConnection": MessageLookupByLibrary.simpleMessage("Connection"),
    "addonDatabase": MessageLookupByLibrary.simpleMessage("Database"),
    "addonDelete": m7,
    "addonDeleteDatabase": MessageLookupByLibrary.simpleMessage(
      "Delete database",
    ),
    "addonEveryAppOfTheProject": MessageLookupByLibrary.simpleMessage(
      "Every app of the project is linked already",
    ),
    "addonForASecondDatabaseCache": MessageLookupByLibrary.simpleMessage(
      "For a second database: CACHE_ gives CACHE_DATABASE_URL",
    ),
    "addonLink": MessageLookupByLibrary.simpleMessage("Link"),
    "addonLinkAnApp": MessageLookupByLibrary.simpleMessage("Link an app"),
    "addonLinkOverridesShared": MessageLookupByLibrary.simpleMessage(
      "An app linked here uses its own link (and prefix) instead.",
    ),
    "addonLinkToAnApp": m8,
    "addonLinkedApps": MessageLookupByLibrary.simpleMessage("Linked apps"),
    "addonLinkedRedeployToApply": m9,
    "addonLowercaseLettersDigitsDashes": MessageLookupByLibrary.simpleMessage(
      "Lowercase letters, digits, dashes",
    ),
    "addonMemoryMib": MessageLookupByLibrary.simpleMessage("Memory (MiB)"),
    "addonNameAlsoTheHostName": MessageLookupByLibrary.simpleMessage(
      "Name (also the host name)",
    ),
    "addonNoAppUsesThisDatabase": MessageLookupByLibrary.simpleMessage(
      "No app uses this database yet.",
    ),
    "addonPostgresqlMysqlMariadbOrRedis": MessageLookupByLibrary.simpleMessage(
      "PostgreSQL, MySQL, MariaDB or Redis with a persistent volume. Link it to an app and it gets DATABASE_URL (or REDIS_URL).",
    ),
    "addonReachableOnlyFromAppsIn": m10,
    "addonSavedTheDatabaseRestartsWith": MessageLookupByLibrary.simpleMessage(
      "Saved; the database restarts with the new resources",
    ),
    "addonShareHelp": MessageLookupByLibrary.simpleMessage(
      "Every app in the project gets the connection variables, including apps you add later. No link needed. Redeploy the apps to apply.",
    ),
    "addonSharePrefix": MessageLookupByLibrary.simpleMessage(
      "Prefix for the shared variables",
    ),
    "addonShareWithProject": MessageLookupByLibrary.simpleMessage(
      "Share with the whole project",
    ),
    "addonSharedAs": m11,
    "addonSharedSaved": MessageLookupByLibrary.simpleMessage(
      "Saved. Redeploy the apps to apply.",
    ),
    "addonShowCredentials": MessageLookupByLibrary.simpleMessage(
      "Show credentials",
    ),
    "addonStorageMib": MessageLookupByLibrary.simpleMessage("Storage (MiB)"),
    "addonTabEvents": MessageLookupByLibrary.simpleMessage("Events"),
    "addonTabLogs": MessageLookupByLibrary.simpleMessage("Logs"),
    "addonTabMetrics": MessageLookupByLibrary.simpleMessage("Metrics"),
    "addonTabOverview": MessageLookupByLibrary.simpleMessage("Overview"),
    "addonTabTerminal": MessageLookupByLibrary.simpleMessage("Terminal"),
    "addonTheDatabaseAndAllIts": MessageLookupByLibrary.simpleMessage(
      "The database and all its data are deleted for good. Linked apps lose their connection settings with their next deploy.",
    ),
    "addonUnlink": MessageLookupByLibrary.simpleMessage("Unlink"),
    "addonVariablePrefixOptional": MessageLookupByLibrary.simpleMessage(
      "Variable prefix (optional)",
    ),
    "alertAddChannel": MessageLookupByLibrary.simpleMessage("Add channel"),
    "alertAlertRulesSaved": MessageLookupByLibrary.simpleMessage(
      "Alert rules saved",
    ),
    "alertAlertsWillNoLongerBe": MessageLookupByLibrary.simpleMessage(
      "Alerts will no longer be sent there.",
    ),
    "alertAllAlerts": MessageLookupByLibrary.simpleMessage("All alerts"),
    "alertAndMore": m12,
    "alertChangingRulesNeedsTheSettings": MessageLookupByLibrary.simpleMessage(
      "Changing rules needs the settings.manage permission.",
    ),
    "alertChannels": MessageLookupByLibrary.simpleMessage("Channels"),
    "alertDelete": m13,
    "alertEncryption": MessageLookupByLibrary.simpleMessage("Encryption"),
    "alertEnterANumber": m14,
    "alertForAtLeast": MessageLookupByLibrary.simpleMessage("For at least"),
    "alertGetNotifiedWhenAppsGo": MessageLookupByLibrary.simpleMessage(
      "Get notified when apps go down, servers run out of resources or certificates are about to expire.",
    ),
    "alertMarkAllRead": MessageLookupByLibrary.simpleMessage("Mark all read"),
    "alertMarkRead": MessageLookupByLibrary.simpleMessage("Mark as read"),
    "alertMarkUnread": MessageLookupByLibrary.simpleMessage("Mark as unread"),
    "alertNoChannelsYet": MessageLookupByLibrary.simpleMessage(
      "No channels yet",
    ),
    "alertNoNotificationChannelYetAlerts": MessageLookupByLibrary.simpleMessage(
      "No notification channel yet: alerts only show up here. Add ntfy, Telegram, email or a webhook under Channels.",
    ),
    "alertNonePort25LocalRelays": MessageLookupByLibrary.simpleMessage(
      "None (port 25, local relays only)",
    ),
    "alertNotifications": MessageLookupByLibrary.simpleMessage("Notifications"),
    "alertNotificationsLinkToTheAffected": MessageLookupByLibrary.simpleMessage(
      "Notifications link to the affected app. Leave empty for no links.",
    ),
    "alertNtfyIsTheQuickestInstall": MessageLookupByLibrary.simpleMessage(
      "ntfy is the quickest: install the ntfy app, subscribe to a topic name only you know, and add it here.",
    ),
    "alertPanelAddressForLinks": MessageLookupByLibrary.simpleMessage(
      "Panel address for links",
    ),
    "alertReadLabel": MessageLookupByLibrary.simpleMessage("read"),
    "alertRemindMeWhileAnAlert": MessageLookupByLibrary.simpleMessage(
      "Remind me while an alert keeps firing",
    ),
    "alertSendATest": MessageLookupByLibrary.simpleMessage("Send a test"),
    "alertSendWithThePanelS": MessageLookupByLibrary.simpleMessage(
      "Send with the panel\'s email settings",
    ),
    "alertSettingsEmailTurnOffTo": MessageLookupByLibrary.simpleMessage(
      "Settings → Email; turn off to use another SMTP server",
    ),
    "alertStarttlsPort587": MessageLookupByLibrary.simpleMessage(
      "STARTTLS (port 587)",
    ),
    "alertTestMessageSentTo": m15,
    "alertTlsPort465": MessageLookupByLibrary.simpleMessage("TLS (port 465)"),
    "app0NoLimitTimeout60": MessageLookupByLibrary.simpleMessage(
      "0 = no limit (timeout: 60 s). Above the rate limit a client gets 429 (Too Many Requests); one that keeps going is blocked for a while (Gateway → Blocked IPs). One page load can make dozens of requests. Older gateways block for 24 h at once and need at least 10.",
    ),
    "app1Year": MessageLookupByLibrary.simpleMessage("1 year"),
    "app2Years": MessageLookupByLibrary.simpleMessage("2 years"),
    "app5MinutesTesting": MessageLookupByLibrary.simpleMessage(
      "5 minutes (testing)",
    ),
    "app6Months": MessageLookupByLibrary.simpleMessage("6 months"),
    "appAddDomain": MessageLookupByLibrary.simpleMessage("Add domain"),
    "appAddFile": MessageLookupByLibrary.simpleMessage("Add file"),
    "appAddRedirectRule": MessageLookupByLibrary.simpleMessage(
      "Add redirect rule",
    ),
    "appAddVariable": MessageLookupByLibrary.simpleMessage("Add variable"),
    "appAddVariables": MessageLookupByLibrary.simpleMessage("Add variables"),
    "appAddVolume": MessageLookupByLibrary.simpleMessage("Add volume"),
    "appAllowCookiesCredentials": MessageLookupByLibrary.simpleMessage(
      "Allow cookies / credentials",
    ),
    "appAllowedIps": MessageLookupByLibrary.simpleMessage("Allowed IPs"),
    "appAllowedOrigins": MessageLookupByLibrary.simpleMessage(
      "Allowed origins",
    ),
    "appAlsoAvailableToTheApp": MessageLookupByLibrary.simpleMessage(
      "Also available to the app as \$PORT",
    ),
    "appAlsoRedirectFromAndKeep": MessageLookupByLibrary.simpleMessage(
      "Also redirect /from/… and keep the rest of the path",
    ),
    "appAlsoWwwRedirectsHere": MessageLookupByLibrary.simpleMessage(
      "Also www. (redirects here)",
    ),
    "appAnyPublicImageOrA": MessageLookupByLibrary.simpleMessage(
      "Any public image, or a private one with a login under Settings → Registries. Pin a version tag (not :latest) so deploys and rollbacks are reproducible.",
    ),
    "appAuto3000032767": MessageLookupByLibrary.simpleMessage(
      "auto (30000–32767)",
    ),
    "appBackToTheProject": MessageLookupByLibrary.simpleMessage(
      "Back to the project",
    ),
    "appBaseImageStopsRightAway": MessageLookupByLibrary.simpleMessage(
      "This is a base OS image: its default command is a shell that exits at once, so the app would keep restarting. Give it a long-running command, or deploy an image that runs a server.",
    ),
    "appBranch": MessageLookupByLibrary.simpleMessage("Branch"),
    "appBuildAndDeployOnEvery": MessageLookupByLibrary.simpleMessage(
      "Build and deploy on every push",
    ),
    "appBuildDeploy": MessageLookupByLibrary.simpleMessage("Build & deploy"),
    "appBuildFromAGitRepository": MessageLookupByLibrary.simpleMessage(
      "Build from a Git repository instead",
    ),
    "appBuildFromGit": MessageLookupByLibrary.simpleMessage("Build from Git"),
    "appBuildStartedItDeploysWhen": m16,
    "appBuilds": MessageLookupByLibrary.simpleMessage("Builds"),
    "appCancelBuild": MessageLookupByLibrary.simpleMessage("Cancel build"),
    "appCancelUpload": MessageLookupByLibrary.simpleMessage("Cancel upload"),
    "appChangeTheseOnTheSettings": MessageLookupByLibrary.simpleMessage(
      "Change these on the Settings tab, variables and files on Environment.",
    ),
    "appChangedAreSavedButNot": m17,
    "appChangesApplyToTheRunning": MessageLookupByLibrary.simpleMessage(
      "Changes apply to the running app at once. New files get the owner of their folder.",
    ),
    "appChangesInsideTheContainerAre": MessageLookupByLibrary.simpleMessage(
      "Changes inside the container are lost on restart, except on volumes. Every session is recorded in the audit log.",
    ),
    "appChangingSettingsNeedsTheApp": MessageLookupByLibrary.simpleMessage(
      "Changing settings needs the app.edit permission.",
    ),
    "appCommands": MessageLookupByLibrary.simpleMessage("Commands"),
    "appCompressTextResponsesGzip": MessageLookupByLibrary.simpleMessage(
      "Compress text responses (gzip)",
    ),
    "appConfigFiles": MessageLookupByLibrary.simpleMessage("Config files"),
    "appConfiguration": MessageLookupByLibrary.simpleMessage("Configuration"),
    "appContainerPort": MessageLookupByLibrary.simpleMessage("Container port"),
    "appContent": MessageLookupByLibrary.simpleMessage("Content"),
    "appCopied": m18,
    "appCopyApp": MessageLookupByLibrary.simpleMessage("Copy app…"),
    "appCouldNotDiscard": m19,
    "appCpuMillicores": MessageLookupByLibrary.simpleMessage(
      "CPU (millicores)",
    ),
    "appCrashLog": MessageLookupByLibrary.simpleMessage("Crash log"),
    "appDatabaseUrlPostgresLogLevel": MessageLookupByLibrary.simpleMessage(
      "DATABASE_URL=postgres://…\nLOG_LEVEL=info",
    ),
    "appDatabases": MessageLookupByLibrary.simpleMessage("Databases"),
    "appDefault": MessageLookupByLibrary.simpleMessage("Default"),
    "appDefaultDomainWithALet": MessageLookupByLibrary.simpleMessage(
      "default domain, with a Let\'s Encrypt certificate",
    ),
    "appDelete": m20,
    "appDelete2": m21,
    "appDeleteApp": MessageLookupByLibrary.simpleMessage("Delete app…"),
    "appDeleteData": MessageLookupByLibrary.simpleMessage("Delete data"),
    "appDeletePreview": m22,
    "appDeletePreviewHelp": MessageLookupByLibrary.simpleMessage(
      "A new commit to the pull request creates it again.",
    ),
    "appDeleteTheDataOf": m23,
    "appDeploy": MessageLookupByLibrary.simpleMessage("Deploy"),
    "appDeployANewVersion": MessageLookupByLibrary.simpleMessage(
      "Deploy a new version",
    ),
    "appDeployAnotherImage": MessageLookupByLibrary.simpleMessage(
      "Deploy another image",
    ),
    "appDeployAnyway": MessageLookupByLibrary.simpleMessage("Deploy anyway"),
    "appDeployImage": MessageLookupByLibrary.simpleMessage("Deploy image"),
    "appDeployInProgress": MessageLookupByLibrary.simpleMessage(
      "Deploy in progress",
    ),
    "appDiscard": MessageLookupByLibrary.simpleMessage("Discard"),
    "appDisconnect": MessageLookupByLibrary.simpleMessage("Disconnect"),
    "appDockerfile": MessageLookupByLibrary.simpleMessage("Dockerfile"),
    "appDownload": MessageLookupByLibrary.simpleMessage("Download"),
    "appEG15Every15": MessageLookupByLibrary.simpleMessage(
      "e.g. */15 * * * * = every 15 minutes, @daily",
    ),
    "appEGMigrateUp": MessageLookupByLibrary.simpleMessage("e.g. ./migrate up"),
    "appEmptyFolder": MessageLookupByLibrary.simpleMessage("Empty folder"),
    "appEnvironmentVariables": MessageLookupByLibrary.simpleMessage(
      "Environment variables",
    ),
    "appEverythingStoredOnThisVolume": MessageLookupByLibrary.simpleMessage(
      "Everything stored on this volume is deleted for good.",
    ),
    "appExistingImage": MessageLookupByLibrary.simpleMessage("Existing image"),
    "appFiles": MessageLookupByLibrary.simpleMessage("Files"),
    "appFiles2": m24,
    "appFilesUpTo64Mb": MessageLookupByLibrary.simpleMessage(
      "Files up to 64 MB can be uploaded here",
    ),
    "appFixedOnceTheVolumeExists": MessageLookupByLibrary.simpleMessage(
      "Fixed once the volume exists.",
    ),
    "appGeneral": MessageLookupByLibrary.simpleMessage("General"),
    "appGhcrIoAcmeApp1": MessageLookupByLibrary.simpleMessage(
      "ghcr.io/acme/app:1.2.3",
    ),
    "appGhcrIoAcmeShop1": MessageLookupByLibrary.simpleMessage(
      "ghcr.io/acme/shop:1.4.2",
    ),
    "appGithubAFineGrainedToken": MessageLookupByLibrary.simpleMessage(
      "GitHub: a fine-grained token with Contents: read. GitLab: read_repository.",
    ),
    "appGithubSettingsWebhooksAddWebhook": MessageLookupByLibrary.simpleMessage(
      "GitHub: Settings → Webhooks → Add webhook, content type application/json, this secret. GitLab: Webhooks, secret token = this secret, push events. Gitea/Forgejo: the same as GitHub. For preview environments also send pull request (GitLab: merge request) events.",
    ),
    "appHiddenInThePanelUnless": MessageLookupByLibrary.simpleMessage(
      "Hidden in the panel unless revealed (always encrypted at rest)",
    ),
    "appHide": MessageLookupByLibrary.simpleMessage("Hide"),
    "appHstsBrowsersOnlyUseHttps": MessageLookupByLibrary.simpleMessage(
      "HSTS (browsers only use HTTPS)",
    ),
    "appHtmlCssJsJsonThat": MessageLookupByLibrary.simpleMessage(
      "HTML, CSS, JS, JSON … that the app does not compress itself",
    ),
    "appHttp": MessageLookupByLibrary.simpleMessage("HTTP"),
    "appHttpsLetSEncrypt": MessageLookupByLibrary.simpleMessage(
      "HTTPS (Let\'s Encrypt)",
    ),
    "appImage": MessageLookupByLibrary.simpleMessage("Image"),
    "appIncludeSubdomains": MessageLookupByLibrary.simpleMessage(
      "Include subdomains",
    ),
    "appIsNotATextFile": m25,
    "appIsSavedAsNotDeployed": m26,
    "appKey": MessageLookupByLibrary.simpleMessage("KEY"),
    "appKubernetesEvents": MessageLookupByLibrary.simpleMessage(
      "Kubernetes events",
    ),
    "appLeaveEmptyForTheDefault": MessageLookupByLibrary.simpleMessage(
      "Leave empty for the default",
    ),
    "appLeaveTheWizard": MessageLookupByLibrary.simpleMessage(
      "Leave the wizard?",
    ),
    "appLetOtherSitesCallThis": MessageLookupByLibrary.simpleMessage(
      "Let other sites call this domain from the browser",
    ),
    "appMaxUploadMb": MessageLookupByLibrary.simpleMessage("Max upload MB"),
    "appMemoryMb": MessageLookupByLibrary.simpleMessage("Memory (MB)"),
    "appMore": MessageLookupByLibrary.simpleMessage("More"),
    "appMoreReplicas": MessageLookupByLibrary.simpleMessage("More replicas"),
    "appMountFilesSuchAsApplication": MessageLookupByLibrary.simpleMessage(
      "Mount files such as application.yml, a .env file or a certificate at any path in the container.",
    ),
    "appMountedAt": MessageLookupByLibrary.simpleMessage("Mounted at"),
    "appNameValuePerLineAn": MessageLookupByLibrary.simpleMessage(
      "\"Name: value\" per line; an empty value removes the header.",
    ),
    "appNeedsThePushWebhookIn": MessageLookupByLibrary.simpleMessage(
      "Needs the push webhook in your Git host",
    ),
    "appNetworkAndHealthCheck": MessageLookupByLibrary.simpleMessage(
      "Network and health check",
    ),
    "appNewFolder": MessageLookupByLibrary.simpleMessage("New folder"),
    "appNewOrHttps": MessageLookupByLibrary.simpleMessage("/new or https://…"),
    "appNewSecret": MessageLookupByLibrary.simpleMessage("New secret"),
    "appNewWebhookSecret": MessageLookupByLibrary.simpleMessage(
      "New webhook secret?",
    ),
    "appNewestPod": MessageLookupByLibrary.simpleMessage("Newest pod"),
    "appNoBaseDomainIsSet": MessageLookupByLibrary.simpleMessage(
      "No base domain is set, so the app gets no domain automatically. Set one in Settings, or add a domain on the app page later.",
    ),
    "appNoBuildYet": MessageLookupByLibrary.simpleMessage("No build yet."),
    "appNoDatabaseLinkedAddOne": MessageLookupByLibrary.simpleMessage(
      "No database linked. Add one on the project page and link it to this app.",
    ),
    "appNoDeploymentsYet": MessageLookupByLibrary.simpleMessage(
      "No deployments yet.",
    ),
    "appNoDomainSetABase": MessageLookupByLibrary.simpleMessage(
      "No domain. Set a base domain in Settings to get one automatically; custom domains come with the domains page.",
    ),
    "appNoLimit": MessageLookupByLibrary.simpleMessage("No limit"),
    "appNoPreviews": MessageLookupByLibrary.simpleMessage(
      "No open pull request has a preview.",
    ),
    "appNoVariables": MessageLookupByLibrary.simpleMessage("No variables."),
    "appNoVariablesEveryAppAlso": MessageLookupByLibrary.simpleMessage(
      "No variables. Every app also gets PORT and APP_ARGS.",
    ),
    "appNoVolumes": MessageLookupByLibrary.simpleMessage("No volumes."),
    "appNone": MessageLookupByLibrary.simpleMessage("None"),
    "appNotDeployedYet": MessageLookupByLibrary.simpleMessage(
      "Not deployed yet",
    ),
    "appOff": MessageLookupByLibrary.simpleMessage("Off"),
    "appOneIpOrCidrPer": MessageLookupByLibrary.simpleMessage(
      "One IP or CIDR per line; everyone else gets 403. Empty = open to everyone.",
    ),
    "appOnePerLineOrFor": MessageLookupByLibrary.simpleMessage(
      "One per line, or * for any site (then without credentials).",
    ),
    "appOpenAPort": MessageLookupByLibrary.simpleMessage("Open a port"),
    "appOpenPortsOnTheServer": MessageLookupByLibrary.simpleMessage(
      "Open ports on the server",
    ),
    "appOpenPreview": MessageLookupByLibrary.simpleMessage("Open"),
    "appOpenPullRequest": MessageLookupByLibrary.simpleMessage("Pull request"),
    "appOpeningTheVolume": MessageLookupByLibrary.simpleMessage(
      "Opening the volume…",
    ),
    "appPassedToYourAppAs": MessageLookupByLibrary.simpleMessage(
      "Passed to your app as \$APP_ARGS",
    ),
    "appPasteAEnvFile": MessageLookupByLibrary.simpleMessage(
      "Paste a .env file",
    ),
    "appPasteEnv": MessageLookupByLibrary.simpleMessage("Paste .env"),
    "appPath": MessageLookupByLibrary.simpleMessage("Path"),
    "appPathInTheContainer": MessageLookupByLibrary.simpleMessage(
      "Path in the container",
    ),
    "appPersistentDisksFilesWrittenThere": MessageLookupByLibrary.simpleMessage(
      "Persistent disks: files written there survive restarts and deploys. Everything else in the container is temporary.",
    ),
    "appPointItsDnsARecord": MessageLookupByLibrary.simpleMessage(
      "Point its DNS (A record) to this server first.",
    ),
    "appPortIsOpenOnThe": m27,
    "appPortOpen": MessageLookupByLibrary.simpleMessage("Port open"),
    "appPortYourAppListensOn": MessageLookupByLibrary.simpleMessage(
      "Port your app listens on",
    ),
    "appPreviewDatabases": MessageLookupByLibrary.simpleMessage(
      "Previews use this app\'s databases",
    ),
    "appPreviewDatabasesHelp": MessageLookupByLibrary.simpleMessage(
      "Off: previews start without database links (they must not change real data).",
    ),
    "appPreviewDays": m28,
    "appPreviewEnvironments": MessageLookupByLibrary.simpleMessage(
      "Preview environments",
    ),
    "appPreviewForks": MessageLookupByLibrary.simpleMessage(
      "Also pull requests from forks",
    ),
    "appPreviewForksHelp": MessageLookupByLibrary.simpleMessage(
      "Their code runs with this app\'s variables and secrets.",
    ),
    "appPreviewOf": m29,
    "appPreviewOfHelp": m30,
    "appPreviewTtl": MessageLookupByLibrary.simpleMessage(
      "Delete inactive previews after",
    ),
    "appPreviewTtlNever": MessageLookupByLibrary.simpleMessage("never"),
    "appPreviews": MessageLookupByLibrary.simpleMessage("Previews"),
    "appPreviewsHelp": MessageLookupByLibrary.simpleMessage(
      "Each pull request into this branch gets its own copy of the app at its own domain, rebuilt on new commits and deleted when the pull request closes. Needs the webhook with pull request events.",
    ),
    "appPreviewsSwitch": MessageLookupByLibrary.simpleMessage(
      "A preview for every pull request",
    ),
    "appProjectDatabases": MessageLookupByLibrary.simpleMessage(
      "Project databases",
    ),
    "appPromoteTo": m31,
    "appPushWebhook": MessageLookupByLibrary.simpleMessage("Push webhook"),
    "appReady": m32,
    "appRedeploy": MessageLookupByLibrary.simpleMessage("Redeploy"),
    "appRedeployCurrentVersion": MessageLookupByLibrary.simpleMessage(
      "Redeploy current version",
    ),
    "appRedirectHttpToHttps": MessageLookupByLibrary.simpleMessage(
      "Redirect HTTP to HTTPS",
    ),
    "appRedirectsAccessHeaders": MessageLookupByLibrary.simpleMessage(
      "Redirects, access, headers",
    ),
    "appRemove": m33,
    "appRemove2": m34,
    "appRemoveFromTheAppThe": MessageLookupByLibrary.simpleMessage(
      "Remove from the app (the data is kept until you delete it)",
    ),
    "appRemoveRule": MessageLookupByLibrary.simpleMessage("Remove rule"),
    "appRemoveTheSavedTokenPublic": MessageLookupByLibrary.simpleMessage(
      "Remove the saved token (public repository)",
    ),
    "appRemovedFromTheAppData": MessageLookupByLibrary.simpleMessage(
      "Removed from the app, data still kept",
    ),
    "appRenameMove": MessageLookupByLibrary.simpleMessage("Rename / move"),
    "appReplace": m35,
    "appReplace2": MessageLookupByLibrary.simpleMessage("Replace"),
    "appRepository": MessageLookupByLibrary.simpleMessage("Repository"),
    "appRequestHeadersToTheApp": MessageLookupByLibrary.simpleMessage(
      "Request headers (to the app)",
    ),
    "appRequestsSPerIp": MessageLookupByLibrary.simpleMessage(
      "Requests/s per IP",
    ),
    "appReserveTheseButAllowUsing": MessageLookupByLibrary.simpleMessage(
      "Reserve these, but allow using any free capacity",
    ),
    "appResourcesPerReplica": MessageLookupByLibrary.simpleMessage(
      "Resources per replica",
    ),
    "appResponseHeaders": MessageLookupByLibrary.simpleMessage(
      "Response headers",
    ),
    "appRestart": MessageLookupByLibrary.simpleMessage("Restart"),
    "appRetryUpload": MessageLookupByLibrary.simpleMessage("Retry upload"),
    "appRevealSecrets": MessageLookupByLibrary.simpleMessage("Reveal secrets"),
    "appRoot": MessageLookupByLibrary.simpleMessage("(root)"),
    "appRotate": MessageLookupByLibrary.simpleMessage("Rotate"),
    "appSaveFile": MessageLookupByLibrary.simpleMessage("Save file"),
    "appSaved": m36,
    "appScheduleCron": MessageLookupByLibrary.simpleMessage("Schedule (cron)"),
    "appSecret": MessageLookupByLibrary.simpleMessage("Secret"),
    "appServerPort": MessageLookupByLibrary.simpleMessage("Server port"),
    "appSharedByAllReplicas": MessageLookupByLibrary.simpleMessage(
      "Shared by all replicas",
    ),
    "appSharedWithProject": MessageLookupByLibrary.simpleMessage(
      "shared with the project",
    ),
    "appShopApi": MessageLookupByLibrary.simpleMessage("Shop API"),
    "appSizeMib": MessageLookupByLibrary.simpleMessage("Size (MiB)"),
    "appSource": MessageLookupByLibrary.simpleMessage("Source"),
    "appSpringProfilesActiveProd": MessageLookupByLibrary.simpleMessage(
      "--spring.profiles.active=prod",
    ),
    "appStartCommandHelp": MessageLookupByLibrary.simpleMessage(
      "Runs with sh -c instead of the image\'s own command. Leave empty for the default.",
    ),
    "appStartCommandOptional": MessageLookupByLibrary.simpleMessage(
      "Start command (optional)",
    ),
    "appStepOf": m37,
    "appStorage": MessageLookupByLibrary.simpleMessage("Storage"),
    "appTcp": MessageLookupByLibrary.simpleMessage("TCP"),
    "appTcpUdpPortsReachableFrom": MessageLookupByLibrary.simpleMessage(
      "TCP/UDP ports reachable from outside (MQTT, game servers…). Web traffic goes through domains instead.",
    ),
    "appTheFileDisappearsFromThe": MessageLookupByLibrary.simpleMessage(
      "The file disappears from the containers with the next deploy.",
    ),
    "appTheFileInThisFolder": MessageLookupByLibrary.simpleMessage(
      "The file in this folder is overwritten.",
    ),
    "appTheGatewayStopsServingIt": MessageLookupByLibrary.simpleMessage(
      "The gateway stops serving it.",
    ),
    "appTheOldSecretStopsWorking": MessageLookupByLibrary.simpleMessage(
      "The old secret stops working: update it in your Git host.",
    ),
    "appTheRepositoryNeedsADockerfile": MessageLookupByLibrary.simpleMessage(
      "The repository needs a Dockerfile. Images are built in the cluster with Kaniko and kept in the built-in registry.",
    ),
    "appTheTypeIsFixedOnce": MessageLookupByLibrary.simpleMessage(
      "The type is fixed once the app exists. Discard the app to choose another type.",
    ),
    "appThereAreNoProjectsYet": MessageLookupByLibrary.simpleMessage(
      "There are no projects yet. Create one first.",
    ),
    "appThisCannotBeUndoneBackups": MessageLookupByLibrary.simpleMessage(
      "This cannot be undone (backups of the volume keep older copies).",
    ),
    "appThisFileSuitsBetterThan": m38,
    "appThisRemovesTheAppIts": MessageLookupByLibrary.simpleMessage(
      "This removes the app, its domains, releases and uploaded files from the server. It cannot be undone.",
    ),
    "appTimeoutS": MessageLookupByLibrary.simpleMessage("Timeout s"),
    "appTrafficOnlyReachesAReplica": MessageLookupByLibrary.simpleMessage(
      "Traffic only reaches a replica once its health check passes, so a broken release never replaces a working one.",
    ),
    "appTypeToConfirm": m39,
    "appUdp": MessageLookupByLibrary.simpleMessage("UDP"),
    "appUpload": MessageLookupByLibrary.simpleMessage("Upload"),
    "appUploadedAndChecked": m40,
    "appUploading": m41,
    "appUse": m42,
    "appUseSleepInfinity": MessageLookupByLibrary.simpleMessage(
      "Use sleep infinity",
    ),
    "appUsed": MessageLookupByLibrary.simpleMessage("Used"),
    "appUsedInKubernetesNamesAnd": MessageLookupByLibrary.simpleMessage(
      "Used in Kubernetes names and the default domain",
    ),
    "appVolumeDeleted": m43,
    "appWasCreatedButNotDeployed": m44,
    "appXEnvStaging": MessageLookupByLibrary.simpleMessage("X-Env: staging"),
    "appXFrameOptionsDeny": MessageLookupByLibrary.simpleMessage(
      "X-Frame-Options: DENY",
    ),
    "appYouCanLeaveThisPage": MessageLookupByLibrary.simpleMessage(
      "You can leave this page; the deploy continues on the server.",
    ),
    "appYourOwnVariablesWithThe": MessageLookupByLibrary.simpleMessage(
      "Your own variables with the same names win.",
    ),
    "authAtLeast10Characters": MessageLookupByLibrary.simpleMessage(
      "At least 10 characters",
    ),
    "authBeforeThePanelHasA": MessageLookupByLibrary.simpleMessage(
      "Before the panel has a domain, open an SSH tunnel and use http://localhost:8480.",
    ),
    "authCannotReachThePanel": MessageLookupByLibrary.simpleMessage(
      "Cannot reach the panel",
    ),
    "authChooseAPassword": MessageLookupByLibrary.simpleMessage(
      "Choose a password",
    ),
    "authCode": MessageLookupByLibrary.simpleMessage("Code"),
    "authConnect": MessageLookupByLibrary.simpleMessage("Connect"),
    "authConnectToYourServer": MessageLookupByLibrary.simpleMessage(
      "Connect to your server",
    ),
    "authContinueWith": m45,
    "authCreateOwnerAccount": MessageLookupByLibrary.simpleMessage(
      "Create Owner account",
    ),
    "authEnterThe6DigitCode": MessageLookupByLibrary.simpleMessage(
      "Enter the 6-digit code from your authenticator app, or one of your recovery codes.",
    ),
    "authForgotYourPassword": MessageLookupByLibrary.simpleMessage(
      "Forgot your password?",
    ),
    "authGoToSignIn": MessageLookupByLibrary.simpleMessage("Go to sign-in"),
    "authIfTheAddressHasAn": MessageLookupByLibrary.simpleMessage(
      "If the address has an account, a reset link is on its way.",
    ),
    "authLinkExpired": MessageLookupByLibrary.simpleMessage("Link expired"),
    "authNectarAppsRunsOnYour": MessageLookupByLibrary.simpleMessage(
      "Nectar Apps runs on your own server. Enter the address of its panel.",
    ),
    "authOr": MessageLookupByLibrary.simpleMessage("or"),
    "authPanelAddress": MessageLookupByLibrary.simpleMessage("Panel address"),
    "authPasswordSaved": MessageLookupByLibrary.simpleMessage("Password saved"),
    "authRepeatPassword": MessageLookupByLibrary.simpleMessage(
      "Repeat password",
    ),
    "authRepeatThePassword": MessageLookupByLibrary.simpleMessage(
      "Repeat the password",
    ),
    "authResetYourPassword": MessageLookupByLibrary.simpleMessage(
      "Reset your password",
    ),
    "authSendLink": MessageLookupByLibrary.simpleMessage("Send link"),
    "authSetupToken": MessageLookupByLibrary.simpleMessage("Setup token"),
    "authSignInWithPasskey": MessageLookupByLibrary.simpleMessage(
      "Sign in with a passkey",
    ),
    "authSignInWithYourNew": MessageLookupByLibrary.simpleMessage(
      "Sign in with your new password and your authenticator code.",
    ),
    "authTheInstallerPrintedItTogether": MessageLookupByLibrary.simpleMessage(
      "The installer printed it together with the setup link.",
    ),
    "authUseAnotherServer": MessageLookupByLibrary.simpleMessage(
      "Use another server",
    ),
    "authVerify": MessageLookupByLibrary.simpleMessage("Verify"),
    "authWeEmailYouALink": MessageLookupByLibrary.simpleMessage(
      "We email you a link to choose a new password (if the panel can send emails).",
    ),
    "authWelcomeToNectarApps": MessageLookupByLibrary.simpleMessage(
      "Welcome to Nectar Apps",
    ),
    "authYourName": MessageLookupByLibrary.simpleMessage("Your name"),
    "backupATargetIsWhereBackups": MessageLookupByLibrary.simpleMessage(
      "A target is where backups go: an S3 bucket (AWS, Backblaze B2, R2, MinIO), an SFTP server or a folder on this server.",
    ),
    "backupAddAPlanForThe": MessageLookupByLibrary.simpleMessage(
      "Add a plan for the panel itself (needed to rebuild a lost server), for databases and for apps with volumes.",
    ),
    "backupAddATargetFirst": MessageLookupByLibrary.simpleMessage(
      "Add a target first",
    ),
    "backupAddPlan": MessageLookupByLibrary.simpleMessage("Add plan"),
    "backupAddTarget": MessageLookupByLibrary.simpleMessage("Add target"),
    "backupBackUpNow": MessageLookupByLibrary.simpleMessage("Back up now"),
    "backupBackblazeB2": MessageLookupByLibrary.simpleMessage("Backblaze B2"),
    "backupBackupOfStarted": m46,
    "backupBackupsAreEncryptedWithThe": MessageLookupByLibrary.simpleMessage(
      "Backups are encrypted with the repository password before they leave the server.",
    ),
    "backupBackupsOfVolumesReadFiles": MessageLookupByLibrary.simpleMessage(
      "Backups of volumes read files while the app runs; databases are dumped consistently. Times are UTC.",
    ),
    "backupBeginOpensshPrivateKey": MessageLookupByLibrary.simpleMessage(
      "-----BEGIN OPENSSH PRIVATE KEY-----",
    ),
    "backupChangesSinceThenAreLost": m47,
    "backupCheck": MessageLookupByLibrary.simpleMessage("Check"),
    "backupChecking": m48,
    "backupCopied": MessageLookupByLibrary.simpleMessage("Copied"),
    "backupCopy": MessageLookupByLibrary.simpleMessage("Copy"),
    "backupCronExpressionUtc": MessageLookupByLibrary.simpleMessage(
      "Cron expression (UTC)",
    ),
    "backupCustomCron": MessageLookupByLibrary.simpleMessage("Custom (cron)"),
    "backupDeleteThisPlan": MessageLookupByLibrary.simpleMessage(
      "Delete this plan?",
    ),
    "backupDone": MessageLookupByLibrary.simpleMessage("Done"),
    "backupEdit": MessageLookupByLibrary.simpleMessage("Edit"),
    "backupFolder": MessageLookupByLibrary.simpleMessage("Folder"),
    "backupItHoldsTheRepositoryPassword": MessageLookupByLibrary.simpleMessage(
      "It holds the repository password and access keys: anyone with it and the master key can read your backups.",
    ),
    "backupKeep": MessageLookupByLibrary.simpleMessage("Keep"),
    "backupMinuteHourDayMonthWeekday": MessageLookupByLibrary.simpleMessage(
      "minute hour day month weekday",
    ),
    "backupName": MessageLookupByLibrary.simpleMessage("Name"),
    "backupNext": m49,
    "backupNoBackupHasRunYet": MessageLookupByLibrary.simpleMessage(
      "No backup has run yet.",
    ),
    "backupNoMoreBackupsOfTo": m50,
    "backupNoSnapshotsYet": MessageLookupByLibrary.simpleMessage(
      "No snapshots yet.",
    ),
    "backupNoTargetYet": MessageLookupByLibrary.simpleMessage("No target yet."),
    "backupNotBackedUpYet": MessageLookupByLibrary.simpleMessage(
      "Not backed up yet",
    ),
    "backupNothingIsBackedUpYet": MessageLookupByLibrary.simpleMessage(
      "Nothing is backed up yet",
    ),
    "backupOlderSnapshotsAreRemovedAfter": MessageLookupByLibrary.simpleMessage(
      "Older snapshots are removed after each backup.",
    ),
    "backupReadingTheRepository": MessageLookupByLibrary.simpleMessage(
      "Reading the repository…",
    ),
    "backupRecoveryKit": MessageLookupByLibrary.simpleMessage("Recovery kit"),
    "backupRemove": m51,
    "backupRemove2": MessageLookupByLibrary.simpleMessage("Remove"),
    "backupRestore": m52,
    "backupRestore2": MessageLookupByLibrary.simpleMessage("Restore"),
    "backupRestoreStartedFollowItUnder": MessageLookupByLibrary.simpleMessage(
      "Restore started: follow it under History",
    ),
    "backupSaveThisAsNectarRecovery": MessageLookupByLibrary.simpleMessage(
      "Save this as nectar-recovery-kit.json somewhere safe, together with your master key. On a new server: install.sh --restore-kit nectar-recovery-kit.json --master-key <key>",
    ),
    "backupScheduledBackupsOn": MessageLookupByLibrary.simpleMessage(
      "Scheduled backups on",
    ),
    "backupSftp": MessageLookupByLibrary.simpleMessage("SFTP"),
    "backupSnapshots": m53,
    "backupThePanelForgetsTheTarget": MessageLookupByLibrary.simpleMessage(
      "The panel forgets the target. The backups stay where they are.",
    ),
    "backupTo": MessageLookupByLibrary.simpleMessage("To"),
    "backupVolumesDatabaseDumpsAndThe": MessageLookupByLibrary.simpleMessage(
      "Volumes, database dumps and the panel itself, encrypted with restic to S3, Backblaze B2, SFTP or a folder.",
    ),
    "backupWhat": MessageLookupByLibrary.simpleMessage("What"),
    "backupWhatIsBackedUp": MessageLookupByLibrary.simpleMessage(
      "What is backed up",
    ),
    "backupWhen": MessageLookupByLibrary.simpleMessage("When"),
    "backupWhereBackupsGo": MessageLookupByLibrary.simpleMessage(
      "Where backups go",
    ),
    "backupYourRoleDoesNotInclude": MessageLookupByLibrary.simpleMessage(
      "Your role does not include backup.manage.",
    ),
    "countApps": m54,
    "dashboardAProjectIsASpace": MessageLookupByLibrary.simpleMessage(
      "A project is a space for related apps with its own quota. Create one, then add your first app.",
    ),
    "dashboardContinueSetup": MessageLookupByLibrary.simpleMessage(
      "Continue setup",
    ),
    "dashboardFinishSettingUpDomainHttps": MessageLookupByLibrary.simpleMessage(
      "Finish setting up: domain, HTTPS email and a sample app.",
    ),
    "dashboardNeedAttention": MessageLookupByLibrary.simpleMessage(
      "Need attention",
    ),
    "dashboardNoAppsYet": MessageLookupByLibrary.simpleMessage("No apps yet."),
    "dashboardNothingDeployedYet": MessageLookupByLibrary.simpleMessage(
      "Nothing deployed yet",
    ),
    "domainActions": MessageLookupByLibrary.simpleMessage("Actions"),
    "domainAll": MessageLookupByLibrary.simpleMessage("All"),
    "domainAutomaticDnsRecordsOff": MessageLookupByLibrary.simpleMessage(
      "Automatic DNS records off",
    ),
    "domainCheckDns": MessageLookupByLibrary.simpleMessage("Check DNS"),
    "domainCreateDnsRecordsAutomatically": MessageLookupByLibrary.simpleMessage(
      "Create DNS records automatically",
    ),
    "domainCustom": MessageLookupByLibrary.simpleMessage("Custom"),
    "domainDnsSetup": MessageLookupByLibrary.simpleMessage("DNS setup"),
    "domainEveryDomainOfEveryApp": MessageLookupByLibrary.simpleMessage(
      "Every domain of every app you can see, with its HTTPS and DNS state.",
    ),
    "domainNoDomainsYet": MessageLookupByLibrary.simpleMessage(
      "No domains yet",
    ),
    "domainNothingMatches": MessageLookupByLibrary.simpleMessage(
      "Nothing matches.",
    ),
    "domainOpenApp": MessageLookupByLibrary.simpleMessage("Open app"),
    "domainPointDomainsAtThisServer": MessageLookupByLibrary.simpleMessage(
      "Point domains at this server with these records at your DNS provider:",
    ),
    "domainProblems": MessageLookupByLibrary.simpleMessage("Problems"),
    "domainSearchDomainsAppsProjects": MessageLookupByLibrary.simpleMessage(
      "Search domains, apps, projects",
    ),
    "domainTheRecordServesEveryApp": m55,
    "domainTheServerSPublicIp": MessageLookupByLibrary.simpleMessage(
      "The server\'s public IP is unknown. Set it in the setup wizard (Settings → Domain, HTTPS email and sample app) so the records can be shown and checked.",
    ),
    "domainWebAppsGetADomain": MessageLookupByLibrary.simpleMessage(
      "Web apps get a domain under the base domain when they are created; add your own on the app page.",
    ),
    "gateway1Day": MessageLookupByLibrary.simpleMessage("1 day"),
    "gateway1Hour": MessageLookupByLibrary.simpleMessage("1 hour"),
    "gateway1Week": MessageLookupByLibrary.simpleMessage("1 week"),
    "gatewayAWildcardCertificateExampleCom": MessageLookupByLibrary.simpleMessage(
      "A wildcard certificate (*.example.com) is proved over DNS: the gateway adds a TXT record through your DNS provider. Apps under it get HTTPS the moment they are created.",
    ),
    "gatewayAboveADomainSRate": MessageLookupByLibrary.simpleMessage(
      "Above a domain\'s rate limit a client gets 429 and may retry a second later. A client that keeps going (more than ten times the limit refused within a minute) is blocked:",
    ),
    "gatewayAllDomains": MessageLookupByLibrary.simpleMessage("All domains"),
    "gatewayAlsoTheDomainItselfExample": MessageLookupByLibrary.simpleMessage(
      "Also the domain itself (example.com)",
    ),
    "gatewayAny": MessageLookupByLibrary.simpleMessage("Any"),
    "gatewayApiToken": MessageLookupByLibrary.simpleMessage("API token"),
    "gatewayBlock": MessageLookupByLibrary.simpleMessage("Block"),
    "gatewayBlock2": m56,
    "gatewayBlockAnIp": MessageLookupByLibrary.simpleMessage("Block an IP"),
    "gatewayBlockIp": MessageLookupByLibrary.simpleMessage("Block IP"),
    "gatewayBlockedIps": MessageLookupByLibrary.simpleMessage("Blocked IPs"),
    "gatewayClientIp": MessageLookupByLibrary.simpleMessage("Client IP"),
    "gatewayDelete": m57,
    "gatewayDomain": MessageLookupByLibrary.simpleMessage("Domain"),
    "gatewayDomainsOfYourAppsAppear": MessageLookupByLibrary.simpleMessage(
      "Domains of your apps appear here.",
    ),
    "gatewayExampleCom": MessageLookupByLibrary.simpleMessage("*.example.com"),
    "gatewayFor": MessageLookupByLibrary.simpleMessage("For"),
    "gatewayGet": m58,
    "gatewayHostsItServesFallBack": MessageLookupByLibrary.simpleMessage(
      "Hosts it serves fall back to their own Let\'s Encrypt certificates (ordered per host).",
    ),
    "gatewayInCloudflareMyProfileApi": MessageLookupByLibrary.simpleMessage(
      "In Cloudflare: My Profile → API Tokens → Create Token → \"Edit zone DNS\", limited to your zone (permissions Zone:Read and DNS:Edit). The token is stored in the gateway and never shown again.",
    ),
    "gatewayIpAddress": MessageLookupByLibrary.simpleMessage("IP address"),
    "gatewayLatestRequests": MessageLookupByLibrary.simpleMessage(
      "Latest requests",
    ),
    "gatewayLetSEncryptCertificatesRenew": MessageLookupByLibrary.simpleMessage(
      "Let\'s Encrypt certificates renew automatically about 30 days before they expire; the \"Certificate expiring\" alert warns when one does not.",
    ),
    "gatewayMs": m59,
    "gatewayNectarGatewayServesEveryDomain": MessageLookupByLibrary.simpleMessage(
      "nectar-gateway serves every domain on ports 80/443: routes, certificates, rate limits and blocked IPs.",
    ),
    "gatewayNoRequestsMatchTheGateway": MessageLookupByLibrary.simpleMessage(
      "No requests match. The gateway keeps the latest requests in memory (ACCESS_LOG_SIZE, 2000 by default).",
    ),
    "gatewayNoRoutes": MessageLookupByLibrary.simpleMessage("No routes"),
    "gatewayNobodyIsBlockedTheGateway": MessageLookupByLibrary.simpleMessage(
      "Nobody is blocked. The gateway blocks scanners (requests for .env, wp-login …) by itself.",
    ),
    "gatewayOneCertificateForYourDomain": MessageLookupByLibrary.simpleMessage(
      "One certificate for *.your-domain serves every app at once, with no Let\'s Encrypt order per app. This needs nectar-gateway 1.2.2 or newer: update it under Settings → Updates.",
    ),
    "gatewayOrder": MessageLookupByLibrary.simpleMessage("Order"),
    "gatewayOtherDomain": MessageLookupByLibrary.simpleMessage("Other domain…"),
    "gatewayRateLimits": MessageLookupByLibrary.simpleMessage("Rate limits"),
    "gatewayRateLimitsTimeoutsAndBody": MessageLookupByLibrary.simpleMessage(
      "Rate limits, timeouts and body limits are set per domain on the app page (Domains → edit).",
    ),
    "gatewayReasonOptional": MessageLookupByLibrary.simpleMessage(
      "Reason (optional)",
    ),
    "gatewayRemoveTheDnsProvider": MessageLookupByLibrary.simpleMessage(
      "Remove the DNS provider?",
    ),
    "gatewayRenewNow": MessageLookupByLibrary.simpleMessage("Renew now"),
    "gatewayStatus": MessageLookupByLibrary.simpleMessage("Status"),
    "gatewayTheGatewaySLetS": MessageLookupByLibrary.simpleMessage(
      "The gateway\'s Let\'s Encrypt client is off: set the Let\'s Encrypt email (Settings → Gateway) first.",
    ),
    "gatewayThisNectarGatewayBlocksA": MessageLookupByLibrary.simpleMessage(
      "This nectar-gateway blocks a client over a rate limit for 24 hours. Update it to answer 429 instead.",
    ),
    "gatewayUnblock": MessageLookupByLibrary.simpleMessage("Unblock"),
    "gatewayUntilUnblocked": MessageLookupByLibrary.simpleMessage(
      "until unblocked",
    ),
    "gatewayWildcardCertificate": MessageLookupByLibrary.simpleMessage(
      "Wildcard certificate",
    ),
    "gatewayWildcardCertificates": MessageLookupByLibrary.simpleMessage(
      "Wildcard certificates",
    ),
    "gatewayWildcardCertificatesStopRenewingUntil":
        MessageLookupByLibrary.simpleMessage(
          "Wildcard certificates stop renewing until you set it again.",
        ),
    "gatewayYourRoleDoesNotInclude": MessageLookupByLibrary.simpleMessage(
      "Your role does not include server.view.",
    ),
    "goToDashboard": MessageLookupByLibrary.simpleMessage(
      "Go to the dashboard",
    ),
    "languageEnglish": MessageLookupByLibrary.simpleMessage("English"),
    "languageSystem": MessageLookupByLibrary.simpleMessage("System"),
    "languageUkrainian": MessageLookupByLibrary.simpleMessage("Українська"),
    "loginButton": MessageLookupByLibrary.simpleMessage("Sign in"),
    "loginEmail": MessageLookupByLibrary.simpleMessage("Email"),
    "loginPassword": MessageLookupByLibrary.simpleMessage("Password"),
    "loginTitle": MessageLookupByLibrary.simpleMessage("Sign in"),
    "logout": MessageLookupByLibrary.simpleMessage("Sign out"),
    "navAlerts": MessageLookupByLibrary.simpleMessage("Alerts"),
    "navBackups": MessageLookupByLibrary.simpleMessage("Backups"),
    "navDashboard": MessageLookupByLibrary.simpleMessage("Dashboard"),
    "navDomains": MessageLookupByLibrary.simpleMessage("Domains"),
    "navProjects": MessageLookupByLibrary.simpleMessage("Projects"),
    "navServer": MessageLookupByLibrary.simpleMessage("Server"),
    "navSettings": MessageLookupByLibrary.simpleMessage("Settings"),
    "newApp": MessageLookupByLibrary.simpleMessage("New app"),
    "noAccess": MessageLookupByLibrary.simpleMessage("No access"),
    "observeAllPods": MessageLookupByLibrary.simpleMessage("All pods"),
    "observeBeforeTheLastRestart": m60,
    "observeClear": MessageLookupByLibrary.simpleMessage("Clear"),
    "observeCopiedLines": m61,
    "observeCopiedLines2": m62,
    "observeCopyShownLines": MessageLookupByLibrary.simpleMessage(
      "Copy shown lines",
    ),
    "observeDownloadLast5000Lines": MessageLookupByLibrary.simpleMessage(
      "Download (last 5000 lines)",
    ),
    "observeErrors5xx": MessageLookupByLibrary.simpleMessage("Errors (5xx)"),
    "observeEverythingKept": MessageLookupByLibrary.simpleMessage(
      "Everything kept",
    ),
    "observeFilter": MessageLookupByLibrary.simpleMessage("Filter"),
    "observeHistory": MessageLookupByLibrary.simpleMessage("History"),
    "observeLast24Hours": MessageLookupByLibrary.simpleMessage("Last 24 hours"),
    "observeLast6Hours": MessageLookupByLibrary.simpleMessage("Last 6 hours"),
    "observeLast7Days": MessageLookupByLibrary.simpleMessage("Last 7 days"),
    "observeLastHour": MessageLookupByLibrary.simpleMessage("Last hour"),
    "observeLive": MessageLookupByLibrary.simpleMessage("Live"),
    "observeNewestFirstLogsAreKept": MessageLookupByLibrary.simpleMessage(
      "Newest first. Logs are kept on the panel for the days set under Server → Disk → Retention.",
    ),
    "observeNoRequestsYet": MessageLookupByLibrary.simpleMessage(
      "No requests yet.",
    ),
    "observeNoRequestsYetTrafficAppears": MessageLookupByLibrary.simpleMessage(
      "No requests yet. Traffic appears once the app has a domain and the gateway routes requests to it.",
    ),
    "observePerSecond": MessageLookupByLibrary.simpleMessage("Per second"),
    "observeRegex": MessageLookupByLibrary.simpleMessage(".* regex"),
    "observeRequested": MessageLookupByLibrary.simpleMessage("Requested"),
    "observeRequests": MessageLookupByLibrary.simpleMessage("Requests"),
    "observeSearch": MessageLookupByLibrary.simpleMessage("Search"),
    "observeThePreviousContainerWroteNothing":
        MessageLookupByLibrary.simpleMessage(
          "The previous container wrote nothing.",
        ),
    "observeTotalsAcrossReplicasTheDashed": m63,
    "observeTrafficLastHour": MessageLookupByLibrary.simpleMessage(
      "Traffic · last hour",
    ),
    "onboardingAtYourDnsProviderAdd": MessageLookupByLibrary.simpleMessage(
      "At your DNS provider, add one record:",
    ),
    "onboardingBaseDomain": MessageLookupByLibrary.simpleMessage("Base domain"),
    "onboardingContinue": MessageLookupByLibrary.simpleMessage("Continue"),
    "onboardingDeploySample": MessageLookupByLibrary.simpleMessage(
      "Deploy sample",
    ),
    "onboardingEmailForLetSEncrypt": MessageLookupByLibrary.simpleMessage(
      "Email for Let\'s Encrypt",
    ),
    "onboardingImage50mCpu64Mb": m64,
    "onboardingLeaveItEmptyToRegister": MessageLookupByLibrary.simpleMessage(
      "Leave it empty to register without an email.",
    ),
    "onboardingOnlyAnOwnerCanSet": MessageLookupByLibrary.simpleMessage(
      "Only an Owner can set up the panel",
    ),
    "onboardingOnlyUsedForExpiryWarnings": MessageLookupByLibrary.simpleMessage(
      "Only used for expiry warnings",
    ),
    "onboardingSetUpNectarApps": MessageLookupByLibrary.simpleMessage(
      "Set up Nectar Apps",
    ),
    "onboardingSkip": MessageLookupByLibrary.simpleMessage("Skip"),
    "onboardingSkipSetupForNow": MessageLookupByLibrary.simpleMessage(
      "Skip setup for now",
    ),
    "onboardingThreeShortStepsYouCan": MessageLookupByLibrary.simpleMessage(
      "Three short steps. You can change everything later in Settings.",
    ),
    "oneClickApp": MessageLookupByLibrary.simpleMessage("One-click app"),
    "pageNotFound": MessageLookupByLibrary.simpleMessage("Page not found"),
    "projectActivity": MessageLookupByLibrary.simpleMessage("Activity"),
    "projectAdopted": MessageLookupByLibrary.simpleMessage("Adopted"),
    "projectAdoptedFromAnExistingNamespace": MessageLookupByLibrary.simpleMessage(
      "Adopted from an existing namespace: other workloads there are left alone.",
    ),
    "projectAllProjects": MessageLookupByLibrary.simpleMessage("All projects"),
    "projectAllTemplates": MessageLookupByLibrary.simpleMessage("All"),
    "projectApplyNectarYaml": MessageLookupByLibrary.simpleMessage(
      "Apply nectar.yaml",
    ),
    "projectApps": MessageLookupByLibrary.simpleMessage("Apps"),
    "projectBack": MessageLookupByLibrary.simpleMessage("Back"),
    "projectChoose": m65,
    "projectCopiedApplyItWithNectar": m66,
    "projectCopiesTheEnvGroupsThe": MessageLookupByLibrary.simpleMessage(
      "Copies the env groups, the databases (new and empty, with new passwords) and the apps with their settings, variables and config files. Nothing is deployed. Later, \"Promote\" deploys the live build of an app to the same app in the other environment.",
    ),
    "projectCopiesTheSettingsVariablesSecrets":
        MessageLookupByLibrary.simpleMessage(
          "Copies the settings, variables (secrets too), config files and env groups. Volumes start empty; nothing is deployed.",
        ),
    "projectCopy": m67,
    "projectCpuRequested": MessageLookupByLibrary.simpleMessage(
      "CPU requested",
    ),
    "projectCreate": MessageLookupByLibrary.simpleMessage("Create"),
    "projectDeleteTheGroup": m68,
    "projectDeployAJarABinary": MessageLookupByLibrary.simpleMessage(
      "Deploy a JAR, a binary, a static site, Node, Python, PHP or any Docker image.",
    ),
    "projectDeployTheApps": MessageLookupByLibrary.simpleMessage(
      "Deploy the apps",
    ),
    "projectEachProjectHasItsOwn": MessageLookupByLibrary.simpleMessage(
      "Each project has its own Kubernetes namespace and optional quota.",
    ),
    "projectEnvGroups": MessageLookupByLibrary.simpleMessage("Env groups"),
    "projectEnvironmentOf": m69,
    "projectEnvironmentOfTheCopy": MessageLookupByLibrary.simpleMessage(
      "Environment of the copy",
    ),
    "projectExportNectarYaml": MessageLookupByLibrary.simpleMessage(
      "Export nectar.yaml",
    ),
    "projectImport": m70,
    "projectImportDockerComposeYml": MessageLookupByLibrary.simpleMessage(
      "Import docker-compose.yml",
    ),
    "projectImportExport": MessageLookupByLibrary.simpleMessage(
      "Import / export",
    ),
    "projectIntoProject": MessageLookupByLibrary.simpleMessage("Into project"),
    "projectInvalidVariableNameUseLetters": m71,
    "projectItsVariablesAreDeleted": MessageLookupByLibrary.simpleMessage(
      "Its variables are deleted.",
    ),
    "projectMemoryRequested": MessageLookupByLibrary.simpleMessage(
      "Memory requested",
    ),
    "projectMyShop": MessageLookupByLibrary.simpleMessage("My Shop"),
    "projectNewEnvironment": MessageLookupByLibrary.simpleMessage(
      "New environment",
    ),
    "projectNewEnvironmentCopy": MessageLookupByLibrary.simpleMessage(
      "New environment (copy)…",
    ),
    "projectNewGroup": MessageLookupByLibrary.simpleMessage("New group"),
    "projectNewProject": MessageLookupByLibrary.simpleMessage("New project"),
    "projectNoAppsInThisProject": MessageLookupByLibrary.simpleMessage(
      "No apps in this project yet",
    ),
    "projectNoProjectsYet": MessageLookupByLibrary.simpleMessage(
      "No projects yet",
    ),
    "projectOpenTheApp": MessageLookupByLibrary.simpleMessage("Open the app"),
    "projectOpenTheDatabase": MessageLookupByLibrary.simpleMessage(
      "Open the database",
    ),
    "projectOrPasteItBelow": MessageLookupByLibrary.simpleMessage(
      "or paste it below",
    ),
    "projectPasswordsAreGeneratedAndStored": MessageLookupByLibrary.simpleMessage(
      "Passwords are generated and stored as secret variables. The app is deployed right away.",
    ),
    "projectProject": MessageLookupByLibrary.simpleMessage("Project"),
    "projectProjectSlugOfTheCopy": MessageLookupByLibrary.simpleMessage(
      "Project slug of the copy",
    ),
    "projectPromote": MessageLookupByLibrary.simpleMessage("Promote"),
    "projectPromoteTo": m72,
    "projectRequestsOfAllReplicasLive": MessageLookupByLibrary.simpleMessage(
      "Requests of all replicas. Live usage arrives with the metrics view.",
    ),
    "projectResources": MessageLookupByLibrary.simpleMessage("Resources"),
    "projectSaved": MessageLookupByLibrary.simpleMessage("Saved"),
    "projectSearchProjects": MessageLookupByLibrary.simpleMessage(
      "Search projects",
    ),
    "projectSearchTemplates": MessageLookupByLibrary.simpleMessage(
      "Search: MQTT, Kafka, Redis, search…",
    ),
    "projectSharedEnvGroups": MessageLookupByLibrary.simpleMessage(
      "Shared env groups",
    ),
    "projectShowSecretValues": MessageLookupByLibrary.simpleMessage(
      "Show secret values",
    ),
    "projectSlug": MessageLookupByLibrary.simpleMessage("Slug"),
    "projectTheAppGetsEveryVariable": MessageLookupByLibrary.simpleMessage(
      "The app gets every variable of the chosen groups; its own variables win.",
    ),
    "projectTheAppSlugItsDefault": MessageLookupByLibrary.simpleMessage(
      "The app slug; its default domain is <name>-<project>.<base domain>",
    ),
    "projectTheLiveBuildOfIts": m73,
    "projectThisProjectHasNoEnv": MessageLookupByLibrary.simpleMessage(
      "This project has no env groups yet (project page → Env groups).",
    ),
    "projectVariablesSeveralAppsShareA": MessageLookupByLibrary.simpleMessage(
      "Variables several apps share (a region, an API endpoint, a Sentry DSN). Apps choose their groups on the Environment tab.",
    ),
    "savedRedeploy": MessageLookupByLibrary.simpleMessage(
      "Saved. Redeploy the app to apply.",
    ),
    "serverANectarGatewayReplicaStarts": MessageLookupByLibrary.simpleMessage(
      "A nectar-gateway replica starts there (ports 80 and 443) and copies routes and certificates from the main gateway. DNS records managed by the panel then point to every edge node. Open ports 80 and 443 on this machine.",
    ),
    "serverAddANode": MessageLookupByLibrary.simpleMessage("Add a node"),
    "serverAddNode": MessageLookupByLibrary.simpleMessage("Add node"),
    "serverAllowNewPodsUncordon": MessageLookupByLibrary.simpleMessage(
      "Allow new pods (uncordon)",
    ),
    "serverClusterNodesTheirResourcesAnd": MessageLookupByLibrary.simpleMessage(
      "Cluster nodes, their resources and recent warnings.",
    ),
    "serverCopyCommand": MessageLookupByLibrary.simpleMessage("Copy command"),
    "serverCpu": MessageLookupByLibrary.simpleMessage("CPU"),
    "serverDeletesTheContainerImagesThat": MessageLookupByLibrary.simpleMessage(
      "Deletes the container images that no container on the server uses right now: old releases, stopped apps and images pulled ahead of time. They are downloaded again when needed, so the next deploy or start of those apps takes longer.",
    ),
    "serverDisk": MessageLookupByLibrary.simpleMessage("Disk"),
    "serverDrain": m74,
    "serverDrain2": MessageLookupByLibrary.simpleMessage("Drain"),
    "serverDrainAnyway": m75,
    "serverDrainAnyway2": MessageLookupByLibrary.simpleMessage("Drain anyway"),
    "serverDrainMovePodsAway": MessageLookupByLibrary.simpleMessage(
      "Drain (move pods away)",
    ),
    "serverExtraImages": MessageLookupByLibrary.simpleMessage("Extra images"),
    "serverGateway": MessageLookupByLibrary.simpleMessage("Gateway"),
    "serverHistoryLast": m76,
    "serverImageCleanupStarted": MessageLookupByLibrary.simpleMessage(
      "Image cleanup started",
    ),
    "serverImageCleanups": MessageLookupByLibrary.simpleMessage(
      "Image cleanups",
    ),
    "serverIn": MessageLookupByLibrary.simpleMessage("In"),
    "serverItsGatewayReplicaStopsAnd": MessageLookupByLibrary.simpleMessage(
      "Its gateway replica stops and the panel removes the node from the DNS records it manages. Records you created yourself still point there.",
    ),
    "serverK3sDataDirectory": MessageLookupByLibrary.simpleMessage(
      "k3s data directory",
    ),
    "serverKeepAppLogs": MessageLookupByLibrary.simpleMessage("Keep app logs"),
    "serverKeepImagesOnEveryNode": MessageLookupByLibrary.simpleMessage(
      "Keep images on every node",
    ),
    "serverKeptTheLastUploadsPer": m77,
    "serverLiveUsageNeedsMetricsServer": MessageLookupByLibrary.simpleMessage(
      "Live usage needs metrics-server (k3s includes it).",
    ),
    "serverMakeAnEdgeNode": m78,
    "serverMakeEdgeNode": MessageLookupByLibrary.simpleMessage(
      "Make edge node",
    ),
    "serverMemory": MessageLookupByLibrary.simpleMessage("Memory"),
    "serverNoNewPodsCordon": MessageLookupByLibrary.simpleMessage(
      "No new pods (cordon)",
    ),
    "serverNoNewPodsStartThere": MessageLookupByLibrary.simpleMessage(
      "No new pods start there, and its pods move to the other nodes (DaemonSet pods stay). Apps with one replica are briefly unavailable.",
    ),
    "serverNoVolumesYetAppsGet": MessageLookupByLibrary.simpleMessage(
      "No volumes yet. Apps get one under Settings → Volumes; databases have their own.",
    ),
    "serverNodeActions": MessageLookupByLibrary.simpleMessage("Node actions"),
    "serverOnLocalPathStorageThe": MessageLookupByLibrary.simpleMessage(
      "On local-path storage (the k3s default) a size is not enforced: a volume can grow past it until the node\'s disk is full. The \"Volume almost full\" alert warns at 90 %.",
    ),
    "serverOnePerLineEG": MessageLookupByLibrary.simpleMessage(
      "One per line, e.g. postgres:17 (up to 20)",
    ),
    "serverOut": MessageLookupByLibrary.simpleMessage("Out"),
    "serverPanelDataVolume": MessageLookupByLibrary.simpleMessage(
      "Panel data volume",
    ),
    "serverPrePullsBusyboxAndThe": MessageLookupByLibrary.simpleMessage(
      "Pre-pulls busybox and the runtime images your apps use, so new nodes and moved pods start without downloading them. Image cleanup keeps them.",
    ),
    "serverRemoveFromTheCluster": m79,
    "serverRemoveFromTheCluster2": MessageLookupByLibrary.simpleMessage(
      "Remove from the cluster",
    ),
    "serverRemoveImages": MessageLookupByLibrary.simpleMessage("Remove images"),
    "serverRemoveUnusedImages": MessageLookupByLibrary.simpleMessage(
      "Remove unused images?",
    ),
    "serverRetention": MessageLookupByLibrary.simpleMessage("Retention"),
    "serverRunThisAsRootOn": MessageLookupByLibrary.simpleMessage(
      "Run this as root on the new machine (Linux, a fresh install):",
    ),
    "serverServeTrafficTooEdgeNode": MessageLookupByLibrary.simpleMessage(
      "Serve traffic too (edge node)",
    ),
    "serverServerDisk": MessageLookupByLibrary.simpleMessage("Server disk"),
    "serverStop": MessageLookupByLibrary.simpleMessage("Stop"),
    "serverStopServingTrafficEdge": MessageLookupByLibrary.simpleMessage(
      "Stop serving traffic (edge)",
    ),
    "serverStopServingTrafficOn": m80,
    "serverStoresEveryAppSOutput": MessageLookupByLibrary.simpleMessage(
      "Stores every app\'s output on the panel\'s disk, searchable on the Logs tab after restarts and deploys.",
    ),
    "serverTheCommandContainsTheCluster": MessageLookupByLibrary.simpleMessage(
      "The command contains the cluster join token: treat it like a password.",
    ),
    "serverTheNodeObjectIsDeleted": MessageLookupByLibrary.simpleMessage(
      "The node object is deleted. On the machine, run /usr/local/bin/k3s-agent-uninstall.sh to remove k3s.",
    ),
    "serverUsageShowsWhileTheApp": m81,
    "serverUsedToFindCrictlFor": MessageLookupByLibrary.simpleMessage(
      "Used to find crictl for image cleanup",
    ),
    "serverVolumes": MessageLookupByLibrary.simpleMessage("Volumes"),
    "serverWarnings": MessageLookupByLibrary.simpleMessage("Warnings"),
    "serverYourRoleDoesNotInclude": MessageLookupByLibrary.simpleMessage(
      "Your role does not include viewing the server (server.view).",
    ),
    "setting1123456789Android": MessageLookupByLibrary.simpleMessage(
      "1:123456789:android:…",
    ),
    "setting1123456789Ios": MessageLookupByLibrary.simpleMessage(
      "1:123456789:ios:…",
    ),
    "setting1CreateAFirebaseProject": MessageLookupByLibrary.simpleMessage(
      "1. Create a Firebase project (console.firebase.google.com) and add an Android app and/or an Apple app with the Nectar app\'s package / bundle id.\n2. iPhone and Mac: upload your APNs key under Project settings → Cloud Messaging.\n3. Copy the values below from Project settings, then add a \"Mobile app\" channel under Alerts.",
    ),
    "setting1ScanThisCodeWith": MessageLookupByLibrary.simpleMessage(
      "1. Scan this code with your authenticator app.",
    ),
    "setting2EnterThe6Digit": MessageLookupByLibrary.simpleMessage(
      "2. Enter the 6-digit code it shows.",
    ),
    "settingAbout": MessageLookupByLibrary.simpleMessage("About"),
    "settingAccount": MessageLookupByLibrary.simpleMessage("Account"),
    "settingActiveSince": m82,
    "settingAddLogin": MessageLookupByLibrary.simpleMessage("Add login"),
    "settingAddPasskey": MessageLookupByLibrary.simpleMessage("Add passkey"),
    "settingAddProvider": MessageLookupByLibrary.simpleMessage("Add provider"),
    "settingAiza": MessageLookupByLibrary.simpleMessage("AIza…"),
    "settingAllowOnlyTheseIpAddresses": m83,
    "settingAllowedEmailDomains": MessageLookupByLibrary.simpleMessage(
      "Allowed email domains",
    ),
    "settingApiKey": MessageLookupByLibrary.simpleMessage("API key"),
    "settingAppId": MessageLookupByLibrary.simpleMessage("App ID"),
    "settingAppliedAtEverySignIn": MessageLookupByLibrary.simpleMessage(
      "Applied at every sign-in from the provider\'s \"groups\" claim.",
    ),
    "settingAppsCanNoLongerPull": MessageLookupByLibrary.simpleMessage(
      "Apps can no longer pull private images from it.",
    ),
    "settingAsInXcodeNeededFor": MessageLookupByLibrary.simpleMessage(
      "as in Xcode (needed for iPhone)",
    ),
    "settingBundleId": MessageLookupByLibrary.simpleMessage("Bundle ID"),
    "settingButtonLabel": MessageLookupByLibrary.simpleMessage("Button label"),
    "settingCallbackRedirectUrlToRegister":
        MessageLookupByLibrary.simpleMessage(
          "Callback (redirect) URL to register at the provider:",
        ),
    "settingChanged": m84,
    "settingClientId": MessageLookupByLibrary.simpleMessage("Client ID"),
    "settingClientSecret": MessageLookupByLibrary.simpleMessage(
      "Client secret",
    ),
    "settingCompanySso": MessageLookupByLibrary.simpleMessage("Company SSO"),
    "settingCreateUsersOnTheirFirst": MessageLookupByLibrary.simpleMessage(
      "Create users on their first sign-in",
    ),
    "settingDomainHttpsEmailAndSample": MessageLookupByLibrary.simpleMessage(
      "Domain, HTTPS email and sample app (setup wizard)",
    ),
    "settingEachCodeSignsYouIn": MessageLookupByLibrary.simpleMessage(
      "Each code signs you in once if your phone is not at hand. Store them in your password manager: they are shown only now.",
    ),
    "settingEmailSettingsSaved": MessageLookupByLibrary.simpleMessage(
      "Email settings saved",
    ),
    "settingEmptyOnlyUsersThatAlready": MessageLookupByLibrary.simpleMessage(
      "Empty: only users that already exist can sign in this way (matched by email).",
    ),
    "settingExampleComExampleOrg": MessageLookupByLibrary.simpleMessage(
      "example.com, example.org",
    ),
    "settingForTheNectarCliGithub": MessageLookupByLibrary.simpleMessage(
      "For the nectar CLI, GitHub Actions and scripts",
    ),
    "settingGhcrATokenWithRead": MessageLookupByLibrary.simpleMessage(
      "GHCR: a token with read:packages",
    ),
    "settingGhcrIoDockerIoRegistry": MessageLookupByLibrary.simpleMessage(
      "ghcr.io, docker.io, registry.gitlab.com",
    ),
    "settingGroupRoleOnePerLine": MessageLookupByLibrary.simpleMessage(
      "Group → role (one per line)",
    ),
    "settingISavedThem": MessageLookupByLibrary.simpleMessage("I saved them"),
    "settingId": MessageLookupByLibrary.simpleMessage("ID"),
    "settingInvitePeopleAndGiveThem": MessageLookupByLibrary.simpleMessage(
      "Invite people and give them roles, everywhere or per project",
    ),
    "settingIssuerUrl": MessageLookupByLibrary.simpleMessage("Issuer URL"),
    "settingItsButtonDisappearsFromThe": MessageLookupByLibrary.simpleMessage(
      "Its button disappears from the login page. Users keep their accounts.",
    ),
    "settingLetPeopleSignInWith": MessageLookupByLibrary.simpleMessage(
      "Let people sign in with GitHub, Google or your company\'s identity provider (Authentik, Keycloak, Zitadel, Okta, Entra ID … over OpenID Connect). Works in the web panel.",
    ),
    "settingLicenses": MessageLookupByLibrary.simpleMessage("Licenses"),
    "settingLoginsAreStoredEncryptedAnd": MessageLookupByLibrary.simpleMessage(
      "Logins are stored encrypted and given to every project as an image pull secret; builds use them for base images.",
    ),
    "settingNectarAdminsAdminDevelopersDeveloper":
        MessageLookupByLibrary.simpleMessage(
          "nectar-admins=admin\ndevelopers=developer",
        ),
    "settingNectarAppsPanelExampleCom": MessageLookupByLibrary.simpleMessage(
      "Nectar Apps <panel@example.com>",
    ),
    "settingNectarAppsTechbrainLlcNectar": MessageLookupByLibrary.simpleMessage(
      "Nectar Apps · © TechBrain LLC · Nectar Source License",
    ),
    "settingNeedsAllowedDomains": MessageLookupByLibrary.simpleMessage(
      "Needs allowed domains.",
    ),
    "settingNewRecoveryCodes": MessageLookupByLibrary.simpleMessage(
      "New recovery codes",
    ),
    "settingNoEncryption": MessageLookupByLibrary.simpleMessage(
      "No encryption",
    ),
    "settingNoPrivateRegistryLoginsPublic": MessageLookupByLibrary.simpleMessage(
      "No private registry logins. Public images (Docker Hub, GHCR public packages, …) need none.",
    ),
    "settingNoneAnAdminAssignsRoles": MessageLookupByLibrary.simpleMessage(
      "None (an admin assigns roles)",
    ),
    "settingOnlyRecoveryCodesLeftCreate": m85,
    "settingOnlyTheListedAddressesMay": MessageLookupByLibrary.simpleMessage(
      "Only the listed addresses may use the panel",
    ),
    "settingOptionalAlertsReachTheNectar": MessageLookupByLibrary.simpleMessage(
      "Optional. Alerts reach the Nectar app on Android, iPhone and Mac through your own Firebase project (free). Without it, use ntfy, Telegram or email channels under Alerts.",
    ),
    "settingOrEnterTheKeyBy": MessageLookupByLibrary.simpleMessage(
      "Or enter the key by hand:",
    ),
    "settingOwner": MessageLookupByLibrary.simpleMessage("Owner"),
    "settingPanelAccess": MessageLookupByLibrary.simpleMessage("Panel access"),
    "settingPanelGatewayAndK3sVersions": MessageLookupByLibrary.simpleMessage(
      "Panel, gateway and k3s versions, automatic updates, history and rollback",
    ),
    "settingPasskeyAdded": MessageLookupByLibrary.simpleMessage(
      "Passkey added: sign in with it on the login page",
    ),
    "settingPasskeyDelete": m86,
    "settingPasskeyDeleteHelp": MessageLookupByLibrary.simpleMessage(
      "It can no longer sign in here. Also delete it from your device or password manager.",
    ),
    "settingPasskeyName": MessageLookupByLibrary.simpleMessage("New passkey"),
    "settingPasskeyNameHint": MessageLookupByLibrary.simpleMessage(
      "Name (e.g. MacBook, iPhone, YubiKey)",
    ),
    "settingPasskeyNeverUsed": MessageLookupByLibrary.simpleMessage(
      "not used yet",
    ),
    "settingPasskeySynced": MessageLookupByLibrary.simpleMessage("synced"),
    "settingPasskeyUsed": m87,
    "settingPasskeys": MessageLookupByLibrary.simpleMessage("Passkeys"),
    "settingPasskeysHelp": MessageLookupByLibrary.simpleMessage(
      "Sign in with Touch ID, Face ID, Windows Hello, your phone or a security key — no password and no code needed. A passkey works at this panel address only.",
    ),
    "settingPasskeysWebOnly": MessageLookupByLibrary.simpleMessage(
      "Add and use passkeys in the web panel (a recent Chrome, Edge, Safari or Firefox).",
    ),
    "settingPasswordOrAccessToken": MessageLookupByLibrary.simpleMessage(
      "Password or access token",
    ),
    "settingPermissionSetsCloneABuilt": MessageLookupByLibrary.simpleMessage(
      "Permission sets; clone a built-in role to make your own",
    ),
    "settingPlansTargetsRestoreAndThe": MessageLookupByLibrary.simpleMessage(
      "Plans, targets, restore and the recovery kit",
    ),
    "settingProjectSettingsCloudMessaging":
        MessageLookupByLibrary.simpleMessage(
          "Project settings → Cloud Messaging",
        ),
    "settingProjectSettingsServiceAccountsGenerate":
        MessageLookupByLibrary.simpleMessage(
          "Project settings → Service accounts → Generate new private key. Stored encrypted.",
        ),
    "settingPushNotifications": MessageLookupByLibrary.simpleMessage(
      "Push notifications",
    ),
    "settingRecoveryCodes": MessageLookupByLibrary.simpleMessage(
      "Recovery codes",
    ),
    "settingRegistries": MessageLookupByLibrary.simpleMessage("Registries"),
    "settingRegistry": MessageLookupByLibrary.simpleMessage("Registry"),
    "settingRegistryLogin": MessageLookupByLibrary.simpleMessage(
      "Registry login",
    ),
    "settingRemove": m88,
    "settingRemoveTheLoginFor": m89,
    "settingRoleOfNewUsersEverywhere": MessageLookupByLibrary.simpleMessage(
      "Role of new users (everywhere)",
    ),
    "settingRulesAndNotificationChannels": MessageLookupByLibrary.simpleMessage(
      "Rules and notification channels",
    ),
    "settingSaveAndSendATest": MessageLookupByLibrary.simpleMessage(
      "Save and send a test",
    ),
    "settingSendPushNotifications": MessageLookupByLibrary.simpleMessage(
      "Send push notifications",
    ),
    "settingSendTestToMyDevices": MessageLookupByLibrary.simpleMessage(
      "Send test to my devices",
    ),
    "settingSenderId": MessageLookupByLibrary.simpleMessage("Sender ID"),
    "settingServiceAccountKeyJson": MessageLookupByLibrary.simpleMessage(
      "Service account key (JSON)",
    ),
    "settingSetUp": MessageLookupByLibrary.simpleMessage("Set up"),
    "settingSetUpTwoFactorAuthentication": MessageLookupByLibrary.simpleMessage(
      "Set up two-factor authentication",
    ),
    "settingShowOnTheLoginPage": MessageLookupByLibrary.simpleMessage(
      "Show on the login page",
    ),
    "settingSignInProviders": MessageLookupByLibrary.simpleMessage(
      "Sign-in providers",
    ),
    "settingSignedIn": MessageLookupByLibrary.simpleMessage("Signed in"),
    "settingStarttls": MessageLookupByLibrary.simpleMessage("STARTTLS"),
    "settingTestEmailSentToYour": MessageLookupByLibrary.simpleMessage(
      "Test email sent to your address",
    ),
    "settingTls": MessageLookupByLibrary.simpleMessage("TLS"),
    "settingTurnOff": MessageLookupByLibrary.simpleMessage("Turn off"),
    "settingTwoFactorAuthentication": MessageLookupByLibrary.simpleMessage(
      "Two-factor authentication",
    ),
    "settingTwoFactorAuthenticationIsOff": MessageLookupByLibrary.simpleMessage(
      "Two-factor authentication is off",
    ),
    "settingUsedForPasswordResetsInvitations":
        MessageLookupByLibrary.simpleMessage(
          "Used for password resets, invitations and alert emails.",
        ),
    "settingUserName": MessageLookupByLibrary.simpleMessage("User name"),
    "settingYourPassword": MessageLookupByLibrary.simpleMessage(
      "Your password",
    ),
    "settingsAppearance": MessageLookupByLibrary.simpleMessage("Appearance"),
    "settingsLanguage": MessageLookupByLibrary.simpleMessage("Language"),
    "settingsLanguageHint": MessageLookupByLibrary.simpleMessage(
      "The interface language on this device. Server messages and logs stay in English.",
    ),
    "shellClockAhead": m90,
    "shellClockBehind": m91,
    "shellLater": MessageLookupByLibrary.simpleMessage("Later"),
    "shellReload": MessageLookupByLibrary.simpleMessage("Reload"),
    "statusDeploying": MessageLookupByLibrary.simpleMessage("Deploying"),
    "statusFailed": MessageLookupByLibrary.simpleMessage("Failed"),
    "statusRunning": MessageLookupByLibrary.simpleMessage("Running"),
    "statusStopped": MessageLookupByLibrary.simpleMessage("Stopped"),
    "themeDark": MessageLookupByLibrary.simpleMessage("Dark"),
    "themeLight": MessageLookupByLibrary.simpleMessage("Light"),
    "themeSystem": MessageLookupByLibrary.simpleMessage("System"),
    "updateAllowAgain": MessageLookupByLibrary.simpleMessage("Allow again"),
    "updateAutomaticUpdates": MessageLookupByLibrary.simpleMessage(
      "Automatic updates",
    ),
    "updateBeta": MessageLookupByLibrary.simpleMessage("Beta"),
    "updateChannel": MessageLookupByLibrary.simpleMessage("Channel"),
    "updateCheckEvery": MessageLookupByLibrary.simpleMessage("Check every"),
    "updateCheckNow": MessageLookupByLibrary.simpleMessage("Check now"),
    "updateCheckedForUpdates": MessageLookupByLibrary.simpleMessage(
      "Checked for updates",
    ),
    "updateEachStepTheDatabaseIs": MessageLookupByLibrary.simpleMessage(
      "Each step: the database is backed up, the image is switched, and the panel waits until the new version is healthy (and the gateway serves its canary and keeps every route). If a check fails it rolls back on its own.",
    ),
    "updateEmptyTheOfficialFeedsWhen": MessageLookupByLibrary.simpleMessage(
      "Empty = the official feeds. When a feed cannot be read, the panel asks the GitHub Releases API instead.",
    ),
    "updateFrom": MessageLookupByLibrary.simpleMessage("From"),
    "updateGatewayFeedUrl": MessageLookupByLibrary.simpleMessage(
      "Gateway feed URL",
    ),
    "updateK3sUpgradeToStartedNode": m92,
    "updateMajorAndBreakingReleasesAre": MessageLookupByLibrary.simpleMessage(
      "Major and breaking releases are never installed automatically. A release that failed its checks is not retried.",
    ),
    "updateNoDaysAndNoTimes": MessageLookupByLibrary.simpleMessage(
      "No days and no times = any time. A window like 22:00–02:00 crosses midnight. k3s upgrades use the same window.",
    ),
    "updateNoUpdateHasRunOn": MessageLookupByLibrary.simpleMessage(
      "No update has run on this server yet.",
    ),
    "updatePanelFeedUrl": MessageLookupByLibrary.simpleMessage(
      "Panel feed URL",
    ),
    "updatePolicy": MessageLookupByLibrary.simpleMessage("Policy"),
    "updateRollBack": m93,
    "updateRollBack2": MessageLookupByLibrary.simpleMessage("Roll back"),
    "updateStable": MessageLookupByLibrary.simpleMessage("Stable"),
    "updateStableLatestV131": MessageLookupByLibrary.simpleMessage(
      "stable, latest, v1.31",
    ),
    "updateThePanelNectarGatewayAnd": MessageLookupByLibrary.simpleMessage(
      "The panel, nectar-gateway and k3s. Every update is backed up first, checked after the switch and rolled back when a check fails.",
    ),
    "updateThePanelRestartsNowThis": MessageLookupByLibrary.simpleMessage(
      "The panel restarts now: this page reconnects by itself in a few seconds.",
    ),
    "updateTheSystemUpgradeControllerUpgrades":
        MessageLookupByLibrary.simpleMessage(
          "The system-upgrade-controller upgrades one node at a time: each node is cordoned, k3s restarts with the new version and the node is uncordoned. Pods on a node restart during its upgrade; on a single-node server apps are briefly unavailable.",
        ),
    "updateTimeZone": MessageLookupByLibrary.simpleMessage("Time zone"),
    "updateUpdate": m94,
    "updateUpdate2": MessageLookupByLibrary.simpleMessage("Update…"),
    "updateUpdateK3sTo": m95,
    "updateUpdateSettingsSaved": MessageLookupByLibrary.simpleMessage(
      "Update settings saved",
    ),
    "updateUpdateSources": MessageLookupByLibrary.simpleMessage(
      "Update sources",
    ),
    "updateUpdateStarted": MessageLookupByLibrary.simpleMessage(
      "Update started",
    ),
    "updateUpdates": MessageLookupByLibrary.simpleMessage("Updates"),
    "updateUpdating": m96,
    "updateUpdatingK3sFromHereNeeds": MessageLookupByLibrary.simpleMessage(
      "Updating k3s from here needs the system-upgrade-controller: re-run install.sh with --k3s-upgrades.",
    ),
    "updateUpgrade": MessageLookupByLibrary.simpleMessage("Upgrade"),
    "updateUtcEuropeKyiv": MessageLookupByLibrary.simpleMessage(
      "UTC, Europe/Kyiv",
    ),
    "updateVersion": MessageLookupByLibrary.simpleMessage("Version"),
    "updateVersions": MessageLookupByLibrary.simpleMessage("Versions"),
    "widgetLoading": MessageLookupByLibrary.simpleMessage("Loading"),
    "widgetNoDataYetHistoryFills": MessageLookupByLibrary.simpleMessage(
      "No data yet: history fills in every 15 seconds.",
    ),
    "widgetNothingYet": MessageLookupByLibrary.simpleMessage("Nothing yet."),
    "widgetRetry": MessageLookupByLibrary.simpleMessage("Retry"),
  };
}

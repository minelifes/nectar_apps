// GENERATED CODE - DO NOT MODIFY BY HAND
import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import 'intl/messages_all.dart';

// **************************************************************************
// Generator: Flutter Intl IDE plugin
// Made by Localizely
// **************************************************************************

// ignore_for_file: non_constant_identifier_names, lines_longer_than_80_chars
// ignore_for_file: join_return_with_assignment, prefer_final_in_for_each
// ignore_for_file: avoid_redundant_argument_values, avoid_escaping_inner_quotes

class S {
  S();

  static S? _current;

  static S get current {
    assert(
      _current != null,
      'No instance of S was loaded. Try to initialize the S delegate before accessing S.current.',
    );
    return _current!;
  }

  static const AppLocalizationDelegate delegate = AppLocalizationDelegate();

  static Future<S> load(Locale locale) {
    final name = (locale.countryCode?.isEmpty ?? false)
        ? locale.languageCode
        : locale.toString();
    final localeName = Intl.canonicalizedLocale(name);
    return initializeMessages(localeName).then((_) {
      Intl.defaultLocale = localeName;
      final instance = S();
      S._current = instance;

      return instance;
    });
  }

  static S of(BuildContext context) {
    final instance = S.maybeOf(context);
    assert(
      instance != null,
      'No instance of S present in the widget tree. Did you add S.delegate in localizationsDelegates?',
    );
    return instance!;
  }

  static S? maybeOf(BuildContext context) {
    return Localizations.of<S>(context, S);
  }

  /// `Dashboard`
  String get navDashboard {
    return Intl.message('Dashboard', name: 'navDashboard', desc: '', args: []);
  }

  /// `Projects`
  String get navProjects {
    return Intl.message('Projects', name: 'navProjects', desc: '', args: []);
  }

  /// `Domains`
  String get navDomains {
    return Intl.message('Domains', name: 'navDomains', desc: '', args: []);
  }

  /// `Server`
  String get navServer {
    return Intl.message('Server', name: 'navServer', desc: '', args: []);
  }

  /// `Alerts`
  String get navAlerts {
    return Intl.message('Alerts', name: 'navAlerts', desc: '', args: []);
  }

  /// `Backups`
  String get navBackups {
    return Intl.message('Backups', name: 'navBackups', desc: '', args: []);
  }

  /// `Settings`
  String get navSettings {
    return Intl.message('Settings', name: 'navSettings', desc: '', args: []);
  }

  /// `Cancel`
  String get actionCancel {
    return Intl.message('Cancel', name: 'actionCancel', desc: '', args: []);
  }

  /// `Save`
  String get actionSave {
    return Intl.message('Save', name: 'actionSave', desc: '', args: []);
  }

  /// `Close`
  String get actionClose {
    return Intl.message('Close', name: 'actionClose', desc: '', args: []);
  }

  /// `Delete`
  String get actionDelete {
    return Intl.message('Delete', name: 'actionDelete', desc: '', args: []);
  }

  /// `OK`
  String get actionOk {
    return Intl.message('OK', name: 'actionOk', desc: '', args: []);
  }

  /// `Try again`
  String get actionRetry {
    return Intl.message('Try again', name: 'actionRetry', desc: '', args: []);
  }

  /// `Refresh`
  String get actionRefresh {
    return Intl.message('Refresh', name: 'actionRefresh', desc: '', args: []);
  }

  /// `Appearance`
  String get settingsAppearance {
    return Intl.message(
      'Appearance',
      name: 'settingsAppearance',
      desc: '',
      args: [],
    );
  }

  /// `Dark`
  String get themeDark {
    return Intl.message('Dark', name: 'themeDark', desc: '', args: []);
  }

  /// `Light`
  String get themeLight {
    return Intl.message('Light', name: 'themeLight', desc: '', args: []);
  }

  /// `System`
  String get themeSystem {
    return Intl.message('System', name: 'themeSystem', desc: '', args: []);
  }

  /// `Language`
  String get settingsLanguage {
    return Intl.message(
      'Language',
      name: 'settingsLanguage',
      desc: '',
      args: [],
    );
  }

  /// `System`
  String get languageSystem {
    return Intl.message('System', name: 'languageSystem', desc: '', args: []);
  }

  /// `English`
  String get languageEnglish {
    return Intl.message('English', name: 'languageEnglish', desc: '', args: []);
  }

  /// `Українська`
  String get languageUkrainian {
    return Intl.message(
      'Українська',
      name: 'languageUkrainian',
      desc: '',
      args: [],
    );
  }

  /// `The interface language on this device. Server messages and logs stay in English.`
  String get settingsLanguageHint {
    return Intl.message(
      'The interface language on this device. Server messages and logs stay in English.',
      name: 'settingsLanguageHint',
      desc: '',
      args: [],
    );
  }

  /// `Sign in`
  String get loginTitle {
    return Intl.message('Sign in', name: 'loginTitle', desc: '', args: []);
  }

  /// `Email`
  String get loginEmail {
    return Intl.message('Email', name: 'loginEmail', desc: '', args: []);
  }

  /// `Password`
  String get loginPassword {
    return Intl.message('Password', name: 'loginPassword', desc: '', args: []);
  }

  /// `Sign in`
  String get loginButton {
    return Intl.message('Sign in', name: 'loginButton', desc: '', args: []);
  }

  /// `Sign out`
  String get logout {
    return Intl.message('Sign out', name: 'logout', desc: '', args: []);
  }

  /// `New app`
  String get newApp {
    return Intl.message('New app', name: 'newApp', desc: '', args: []);
  }

  /// `One-click app`
  String get oneClickApp {
    return Intl.message(
      'One-click app',
      name: 'oneClickApp',
      desc: '',
      args: [],
    );
  }

  /// `No access`
  String get noAccess {
    return Intl.message('No access', name: 'noAccess', desc: '', args: []);
  }

  /// `Saved. Redeploy the app to apply.`
  String get savedRedeploy {
    return Intl.message(
      'Saved. Redeploy the app to apply.',
      name: 'savedRedeploy',
      desc: '',
      args: [],
    );
  }

  /// `Page not found`
  String get pageNotFound {
    return Intl.message(
      'Page not found',
      name: 'pageNotFound',
      desc: '',
      args: [],
    );
  }

  /// `Go to the dashboard`
  String get goToDashboard {
    return Intl.message(
      'Go to the dashboard',
      name: 'goToDashboard',
      desc: '',
      args: [],
    );
  }

  /// `Running`
  String get statusRunning {
    return Intl.message('Running', name: 'statusRunning', desc: '', args: []);
  }

  /// `Stopped`
  String get statusStopped {
    return Intl.message('Stopped', name: 'statusStopped', desc: '', args: []);
  }

  /// `Failed`
  String get statusFailed {
    return Intl.message('Failed', name: 'statusFailed', desc: '', args: []);
  }

  /// `Deploying`
  String get statusDeploying {
    return Intl.message(
      'Deploying',
      name: 'statusDeploying',
      desc: '',
      args: [],
    );
  }

  /// `{count, plural, =0{No apps} =1{1 app} other{{count} apps}}`
  String countApps(int count) {
    return Intl.plural(
      count,
      zero: 'No apps',
      one: '1 app',
      other: '$count apps',
      name: 'countApps',
      desc: '',
      args: [count],
    );
  }

  /// `Your role does not include backup.manage.`
  String get backupYourRoleDoesNotInclude {
    return Intl.message(
      'Your role does not include backup.manage.',
      name: 'backupYourRoleDoesNotInclude',
      desc: '',
      args: [],
    );
  }

  /// `Volumes, database dumps and the panel itself, encrypted with restic to S3, Backblaze B2, SFTP or a folder.`
  String get backupVolumesDatabaseDumpsAndThe {
    return Intl.message(
      'Volumes, database dumps and the panel itself, encrypted with restic to S3, Backblaze B2, SFTP or a folder.',
      name: 'backupVolumesDatabaseDumpsAndThe',
      desc: '',
      args: [],
    );
  }

  /// `Backup of {subject} started`
  String backupBackupOfStarted(Object subject) {
    return Intl.message(
      'Backup of $subject started',
      name: 'backupBackupOfStarted',
      desc: '',
      args: [subject],
    );
  }

  /// `Delete this plan?`
  String get backupDeleteThisPlan {
    return Intl.message(
      'Delete this plan?',
      name: 'backupDeleteThisPlan',
      desc: '',
      args: [],
    );
  }

  /// `No more backups of {subject} to {targetName}. Existing snapshots stay in the repository.`
  String backupNoMoreBackupsOfTo(Object subject, Object targetName) {
    return Intl.message(
      'No more backups of $subject to $targetName. Existing snapshots stay in the repository.',
      name: 'backupNoMoreBackupsOfTo',
      desc: '',
      args: [subject, targetName],
    );
  }

  /// `What is backed up`
  String get backupWhatIsBackedUp {
    return Intl.message(
      'What is backed up',
      name: 'backupWhatIsBackedUp',
      desc: '',
      args: [],
    );
  }

  /// `Add plan`
  String get backupAddPlan {
    return Intl.message('Add plan', name: 'backupAddPlan', desc: '', args: []);
  }

  /// `Add a target first`
  String get backupAddATargetFirst {
    return Intl.message(
      'Add a target first',
      name: 'backupAddATargetFirst',
      desc: '',
      args: [],
    );
  }

  /// `A target is where backups go: an S3 bucket (AWS, Backblaze B2, R2, MinIO), an SFTP server or a folder on this server.`
  String get backupATargetIsWhereBackups {
    return Intl.message(
      'A target is where backups go: an S3 bucket (AWS, Backblaze B2, R2, MinIO), an SFTP server or a folder on this server.',
      name: 'backupATargetIsWhereBackups',
      desc: '',
      args: [],
    );
  }

  /// `Nothing is backed up yet`
  String get backupNothingIsBackedUpYet {
    return Intl.message(
      'Nothing is backed up yet',
      name: 'backupNothingIsBackedUpYet',
      desc: '',
      args: [],
    );
  }

  /// `Add a plan for the panel itself (needed to rebuild a lost server), for databases and for apps with volumes.`
  String get backupAddAPlanForThe {
    return Intl.message(
      'Add a plan for the panel itself (needed to rebuild a lost server), for databases and for apps with volumes.',
      name: 'backupAddAPlanForThe',
      desc: '',
      args: [],
    );
  }

  /// `Backups of volumes read files while the app runs; databases are dumped consistently. Times are UTC.`
  String get backupBackupsOfVolumesReadFiles {
    return Intl.message(
      'Backups of volumes read files while the app runs; databases are dumped consistently. Times are UTC.',
      name: 'backupBackupsOfVolumesReadFiles',
      desc: '',
      args: [],
    );
  }

  /// `Not backed up yet`
  String get backupNotBackedUpYet {
    return Intl.message(
      'Not backed up yet',
      name: 'backupNotBackedUpYet',
      desc: '',
      args: [],
    );
  }

  /// `Next: {nextRun}`
  String backupNext(Object nextRun) {
    return Intl.message(
      'Next: $nextRun',
      name: 'backupNext',
      desc: '',
      args: [nextRun],
    );
  }

  /// `Back up now`
  String get backupBackUpNow {
    return Intl.message(
      'Back up now',
      name: 'backupBackUpNow',
      desc: '',
      args: [],
    );
  }

  /// `Edit`
  String get backupEdit {
    return Intl.message('Edit', name: 'backupEdit', desc: '', args: []);
  }

  /// `Restore {shortId}?`
  String backupRestore(Object shortId) {
    return Intl.message(
      'Restore $shortId?',
      name: 'backupRestore',
      desc: '',
      args: [shortId],
    );
  }

  /// `{what} Changes since then are lost.`
  String backupChangesSinceThenAreLost(Object what) {
    return Intl.message(
      '$what Changes since then are lost.',
      name: 'backupChangesSinceThenAreLost',
      desc: '',
      args: [what],
    );
  }

  /// `Restore`
  String get backupRestore2 {
    return Intl.message('Restore', name: 'backupRestore2', desc: '', args: []);
  }

  /// `Restore started: follow it under History`
  String get backupRestoreStartedFollowItUnder {
    return Intl.message(
      'Restore started: follow it under History',
      name: 'backupRestoreStartedFollowItUnder',
      desc: '',
      args: [],
    );
  }

  /// `Snapshots · {subject}`
  String backupSnapshots(Object subject) {
    return Intl.message(
      'Snapshots · $subject',
      name: 'backupSnapshots',
      desc: '',
      args: [subject],
    );
  }

  /// `Reading the repository…`
  String get backupReadingTheRepository {
    return Intl.message(
      'Reading the repository…',
      name: 'backupReadingTheRepository',
      desc: '',
      args: [],
    );
  }

  /// `No snapshots yet.`
  String get backupNoSnapshotsYet {
    return Intl.message(
      'No snapshots yet.',
      name: 'backupNoSnapshotsYet',
      desc: '',
      args: [],
    );
  }

  /// `Checking {tName}…`
  String backupChecking(Object tName) {
    return Intl.message(
      'Checking $tName…',
      name: 'backupChecking',
      desc: '',
      args: [tName],
    );
  }

  /// `Recovery kit`
  String get backupRecoveryKit {
    return Intl.message(
      'Recovery kit',
      name: 'backupRecoveryKit',
      desc: '',
      args: [],
    );
  }

  /// `Save this as nectar-recovery-kit.json somewhere safe, together with your master key. On a new server: install.sh --restore-kit nectar-recovery-kit.json --master-key <key>`
  String get backupSaveThisAsNectarRecovery {
    return Intl.message(
      'Save this as nectar-recovery-kit.json somewhere safe, together with your master key. On a new server: install.sh --restore-kit nectar-recovery-kit.json --master-key <key>',
      name: 'backupSaveThisAsNectarRecovery',
      desc: '',
      args: [],
    );
  }

  /// `It holds the repository password and access keys: anyone with it and the master key can read your backups.`
  String get backupItHoldsTheRepositoryPassword {
    return Intl.message(
      'It holds the repository password and access keys: anyone with it and the master key can read your backups.',
      name: 'backupItHoldsTheRepositoryPassword',
      desc: '',
      args: [],
    );
  }

  /// `Copied`
  String get backupCopied {
    return Intl.message('Copied', name: 'backupCopied', desc: '', args: []);
  }

  /// `Copy`
  String get backupCopy {
    return Intl.message('Copy', name: 'backupCopy', desc: '', args: []);
  }

  /// `Done`
  String get backupDone {
    return Intl.message('Done', name: 'backupDone', desc: '', args: []);
  }

  /// `Remove {tName}?`
  String backupRemove(Object tName) {
    return Intl.message(
      'Remove $tName?',
      name: 'backupRemove',
      desc: '',
      args: [tName],
    );
  }

  /// `The panel forgets the target. The backups stay where they are.`
  String get backupThePanelForgetsTheTarget {
    return Intl.message(
      'The panel forgets the target. The backups stay where they are.',
      name: 'backupThePanelForgetsTheTarget',
      desc: '',
      args: [],
    );
  }

  /// `Remove`
  String get backupRemove2 {
    return Intl.message('Remove', name: 'backupRemove2', desc: '', args: []);
  }

  /// `Where backups go`
  String get backupWhereBackupsGo {
    return Intl.message(
      'Where backups go',
      name: 'backupWhereBackupsGo',
      desc: '',
      args: [],
    );
  }

  /// `Add target`
  String get backupAddTarget {
    return Intl.message(
      'Add target',
      name: 'backupAddTarget',
      desc: '',
      args: [],
    );
  }

  /// `No target yet.`
  String get backupNoTargetYet {
    return Intl.message(
      'No target yet.',
      name: 'backupNoTargetYet',
      desc: '',
      args: [],
    );
  }

  /// `Check`
  String get backupCheck {
    return Intl.message('Check', name: 'backupCheck', desc: '', args: []);
  }

  /// `No backup has run yet.`
  String get backupNoBackupHasRunYet {
    return Intl.message(
      'No backup has run yet.',
      name: 'backupNoBackupHasRunYet',
      desc: '',
      args: [],
    );
  }

  /// `What`
  String get backupWhat {
    return Intl.message('What', name: 'backupWhat', desc: '', args: []);
  }

  /// `To`
  String get backupTo {
    return Intl.message('To', name: 'backupTo', desc: '', args: []);
  }

  /// `When`
  String get backupWhen {
    return Intl.message('When', name: 'backupWhen', desc: '', args: []);
  }

  /// `Custom (cron)`
  String get backupCustomCron {
    return Intl.message(
      'Custom (cron)',
      name: 'backupCustomCron',
      desc: '',
      args: [],
    );
  }

  /// `Cron expression (UTC)`
  String get backupCronExpressionUtc {
    return Intl.message(
      'Cron expression (UTC)',
      name: 'backupCronExpressionUtc',
      desc: '',
      args: [],
    );
  }

  /// `minute hour day month weekday`
  String get backupMinuteHourDayMonthWeekday {
    return Intl.message(
      'minute hour day month weekday',
      name: 'backupMinuteHourDayMonthWeekday',
      desc: '',
      args: [],
    );
  }

  /// `Keep`
  String get backupKeep {
    return Intl.message('Keep', name: 'backupKeep', desc: '', args: []);
  }

  /// `Older snapshots are removed after each backup.`
  String get backupOlderSnapshotsAreRemovedAfter {
    return Intl.message(
      'Older snapshots are removed after each backup.',
      name: 'backupOlderSnapshotsAreRemovedAfter',
      desc: '',
      args: [],
    );
  }

  /// `Scheduled backups on`
  String get backupScheduledBackupsOn {
    return Intl.message(
      'Scheduled backups on',
      name: 'backupScheduledBackupsOn',
      desc: '',
      args: [],
    );
  }

  /// `-----BEGIN OPENSSH PRIVATE KEY-----`
  String get backupBeginOpensshPrivateKey {
    return Intl.message(
      '-----BEGIN OPENSSH PRIVATE KEY-----',
      name: 'backupBeginOpensshPrivateKey',
      desc: '',
      args: [],
    );
  }

  /// `Backblaze B2`
  String get backupBackblazeB2 {
    return Intl.message(
      'Backblaze B2',
      name: 'backupBackblazeB2',
      desc: '',
      args: [],
    );
  }

  /// `SFTP`
  String get backupSftp {
    return Intl.message('SFTP', name: 'backupSftp', desc: '', args: []);
  }

  /// `Folder`
  String get backupFolder {
    return Intl.message('Folder', name: 'backupFolder', desc: '', args: []);
  }

  /// `Name`
  String get backupName {
    return Intl.message('Name', name: 'backupName', desc: '', args: []);
  }

  /// `Backups are encrypted with the repository password before they leave the server.`
  String get backupBackupsAreEncryptedWithThe {
    return Intl.message(
      'Backups are encrypted with the repository password before they leave the server.',
      name: 'backupBackupsAreEncryptedWithThe',
      desc: '',
      args: [],
    );
  }

  /// `Add a node`
  String get serverAddANode {
    return Intl.message(
      'Add a node',
      name: 'serverAddANode',
      desc: '',
      args: [],
    );
  }

  /// `Run this as root on the new machine (Linux, a fresh install):`
  String get serverRunThisAsRootOn {
    return Intl.message(
      'Run this as root on the new machine (Linux, a fresh install):',
      name: 'serverRunThisAsRootOn',
      desc: '',
      args: [],
    );
  }

  /// `The command contains the cluster join token: treat it like a password.`
  String get serverTheCommandContainsTheCluster {
    return Intl.message(
      'The command contains the cluster join token: treat it like a password.',
      name: 'serverTheCommandContainsTheCluster',
      desc: '',
      args: [],
    );
  }

  /// `Copy command`
  String get serverCopyCommand {
    return Intl.message(
      'Copy command',
      name: 'serverCopyCommand',
      desc: '',
      args: [],
    );
  }

  /// `Remove unused images?`
  String get serverRemoveUnusedImages {
    return Intl.message(
      'Remove unused images?',
      name: 'serverRemoveUnusedImages',
      desc: '',
      args: [],
    );
  }

  /// `Deletes the container images that no container on the server uses right now: old releases, stopped apps and images pulled ahead of time. They are downloaded again when needed, so the next deploy or start of those apps takes longer.`
  String get serverDeletesTheContainerImagesThat {
    return Intl.message(
      'Deletes the container images that no container on the server uses right now: old releases, stopped apps and images pulled ahead of time. They are downloaded again when needed, so the next deploy or start of those apps takes longer.',
      name: 'serverDeletesTheContainerImagesThat',
      desc: '',
      args: [],
    );
  }

  /// `Remove images`
  String get serverRemoveImages {
    return Intl.message(
      'Remove images',
      name: 'serverRemoveImages',
      desc: '',
      args: [],
    );
  }

  /// `Image cleanup started`
  String get serverImageCleanupStarted {
    return Intl.message(
      'Image cleanup started',
      name: 'serverImageCleanupStarted',
      desc: '',
      args: [],
    );
  }

  /// `Disk`
  String get serverDisk {
    return Intl.message('Disk', name: 'serverDisk', desc: '', args: []);
  }

  /// `Server disk`
  String get serverServerDisk {
    return Intl.message(
      'Server disk',
      name: 'serverServerDisk',
      desc: '',
      args: [],
    );
  }

  /// `Panel data volume`
  String get serverPanelDataVolume {
    return Intl.message(
      'Panel data volume',
      name: 'serverPanelDataVolume',
      desc: '',
      args: [],
    );
  }

  /// `Kept: the last {keepArtifacts} uploads per app (and every upload a live release uses), {auditDays} days of audit log, {alertDays} days of resolved alerts. Cleanup runs every hour.`
  String serverKeptTheLastUploadsPer(
    Object keepArtifacts,
    Object auditDays,
    Object alertDays,
  ) {
    return Intl.message(
      'Kept: the last $keepArtifacts uploads per app (and every upload a live release uses), $auditDays days of audit log, $alertDays days of resolved alerts. Cleanup runs every hour.',
      name: 'serverKeptTheLastUploadsPer',
      desc: '',
      args: [keepArtifacts, auditDays, alertDays],
    );
  }

  /// `Retention`
  String get serverRetention {
    return Intl.message(
      'Retention',
      name: 'serverRetention',
      desc: '',
      args: [],
    );
  }

  /// `Image cleanups`
  String get serverImageCleanups {
    return Intl.message(
      'Image cleanups',
      name: 'serverImageCleanups',
      desc: '',
      args: [],
    );
  }

  /// `Keep app logs`
  String get serverKeepAppLogs {
    return Intl.message(
      'Keep app logs',
      name: 'serverKeepAppLogs',
      desc: '',
      args: [],
    );
  }

  /// `Stores every app's output on the panel's disk, searchable on the Logs tab after restarts and deploys.`
  String get serverStoresEveryAppSOutput {
    return Intl.message(
      'Stores every app\'s output on the panel\'s disk, searchable on the Logs tab after restarts and deploys.',
      name: 'serverStoresEveryAppSOutput',
      desc: '',
      args: [],
    );
  }

  /// `k3s data directory`
  String get serverK3sDataDirectory {
    return Intl.message(
      'k3s data directory',
      name: 'serverK3sDataDirectory',
      desc: '',
      args: [],
    );
  }

  /// `Used to find crictl for image cleanup`
  String get serverUsedToFindCrictlFor {
    return Intl.message(
      'Used to find crictl for image cleanup',
      name: 'serverUsedToFindCrictlFor',
      desc: '',
      args: [],
    );
  }

  /// `Keep images on every node`
  String get serverKeepImagesOnEveryNode {
    return Intl.message(
      'Keep images on every node',
      name: 'serverKeepImagesOnEveryNode',
      desc: '',
      args: [],
    );
  }

  /// `Pre-pulls busybox and the runtime images your apps use, so new nodes and moved pods start without downloading them. Image cleanup keeps them.`
  String get serverPrePullsBusyboxAndThe {
    return Intl.message(
      'Pre-pulls busybox and the runtime images your apps use, so new nodes and moved pods start without downloading them. Image cleanup keeps them.',
      name: 'serverPrePullsBusyboxAndThe',
      desc: '',
      args: [],
    );
  }

  /// `Extra images`
  String get serverExtraImages {
    return Intl.message(
      'Extra images',
      name: 'serverExtraImages',
      desc: '',
      args: [],
    );
  }

  /// `One per line, e.g. postgres:17 (up to 20)`
  String get serverOnePerLineEG {
    return Intl.message(
      'One per line, e.g. postgres:17 (up to 20)',
      name: 'serverOnePerLineEG',
      desc: '',
      args: [],
    );
  }

  /// `Drain {nodeName}?`
  String serverDrain(Object nodeName) {
    return Intl.message(
      'Drain $nodeName?',
      name: 'serverDrain',
      desc: '',
      args: [nodeName],
    );
  }

  /// `No new pods start there, and its pods move to the other nodes (DaemonSet pods stay). Apps with one replica are briefly unavailable.`
  String get serverNoNewPodsStartThere {
    return Intl.message(
      'No new pods start there, and its pods move to the other nodes (DaemonSet pods stay). Apps with one replica are briefly unavailable.',
      name: 'serverNoNewPodsStartThere',
      desc: '',
      args: [],
    );
  }

  /// `Drain`
  String get serverDrain2 {
    return Intl.message('Drain', name: 'serverDrain2', desc: '', args: []);
  }

  /// `Drain {nodeName} anyway?`
  String serverDrainAnyway(Object nodeName) {
    return Intl.message(
      'Drain $nodeName anyway?',
      name: 'serverDrainAnyway',
      desc: '',
      args: [nodeName],
    );
  }

  /// `Drain anyway`
  String get serverDrainAnyway2 {
    return Intl.message(
      'Drain anyway',
      name: 'serverDrainAnyway2',
      desc: '',
      args: [],
    );
  }

  /// `Make {nodeName} an edge node?`
  String serverMakeAnEdgeNode(Object nodeName) {
    return Intl.message(
      'Make $nodeName an edge node?',
      name: 'serverMakeAnEdgeNode',
      desc: '',
      args: [nodeName],
    );
  }

  /// `A nectar-gateway replica starts there (ports 80 and 443) and copies routes and certificates from the main gateway. DNS records managed by the panel then point to every edge node. Open ports 80 and 443 on this machine.`
  String get serverANectarGatewayReplicaStarts {
    return Intl.message(
      'A nectar-gateway replica starts there (ports 80 and 443) and copies routes and certificates from the main gateway. DNS records managed by the panel then point to every edge node. Open ports 80 and 443 on this machine.',
      name: 'serverANectarGatewayReplicaStarts',
      desc: '',
      args: [],
    );
  }

  /// `Make edge node`
  String get serverMakeEdgeNode {
    return Intl.message(
      'Make edge node',
      name: 'serverMakeEdgeNode',
      desc: '',
      args: [],
    );
  }

  /// `Stop serving traffic on {nodeName}?`
  String serverStopServingTrafficOn(Object nodeName) {
    return Intl.message(
      'Stop serving traffic on $nodeName?',
      name: 'serverStopServingTrafficOn',
      desc: '',
      args: [nodeName],
    );
  }

  /// `Its gateway replica stops and the panel removes the node from the DNS records it manages. Records you created yourself still point there.`
  String get serverItsGatewayReplicaStopsAnd {
    return Intl.message(
      'Its gateway replica stops and the panel removes the node from the DNS records it manages. Records you created yourself still point there.',
      name: 'serverItsGatewayReplicaStopsAnd',
      desc: '',
      args: [],
    );
  }

  /// `Stop`
  String get serverStop {
    return Intl.message('Stop', name: 'serverStop', desc: '', args: []);
  }

  /// `Remove {nodeName} from the cluster?`
  String serverRemoveFromTheCluster(Object nodeName) {
    return Intl.message(
      'Remove $nodeName from the cluster?',
      name: 'serverRemoveFromTheCluster',
      desc: '',
      args: [nodeName],
    );
  }

  /// `The node object is deleted. On the machine, run /usr/local/bin/k3s-agent-uninstall.sh to remove k3s.`
  String get serverTheNodeObjectIsDeleted {
    return Intl.message(
      'The node object is deleted. On the machine, run /usr/local/bin/k3s-agent-uninstall.sh to remove k3s.',
      name: 'serverTheNodeObjectIsDeleted',
      desc: '',
      args: [],
    );
  }

  /// `CPU`
  String get serverCpu {
    return Intl.message('CPU', name: 'serverCpu', desc: '', args: []);
  }

  /// `Memory`
  String get serverMemory {
    return Intl.message('Memory', name: 'serverMemory', desc: '', args: []);
  }

  /// `Live usage needs metrics-server (k3s includes it).`
  String get serverLiveUsageNeedsMetricsServer {
    return Intl.message(
      'Live usage needs metrics-server (k3s includes it).',
      name: 'serverLiveUsageNeedsMetricsServer',
      desc: '',
      args: [],
    );
  }

  /// `Node actions`
  String get serverNodeActions {
    return Intl.message(
      'Node actions',
      name: 'serverNodeActions',
      desc: '',
      args: [],
    );
  }

  /// `Allow new pods (uncordon)`
  String get serverAllowNewPodsUncordon {
    return Intl.message(
      'Allow new pods (uncordon)',
      name: 'serverAllowNewPodsUncordon',
      desc: '',
      args: [],
    );
  }

  /// `No new pods (cordon)`
  String get serverNoNewPodsCordon {
    return Intl.message(
      'No new pods (cordon)',
      name: 'serverNoNewPodsCordon',
      desc: '',
      args: [],
    );
  }

  /// `Drain (move pods away)`
  String get serverDrainMovePodsAway {
    return Intl.message(
      'Drain (move pods away)',
      name: 'serverDrainMovePodsAway',
      desc: '',
      args: [],
    );
  }

  /// `Stop serving traffic (edge)`
  String get serverStopServingTrafficEdge {
    return Intl.message(
      'Stop serving traffic (edge)',
      name: 'serverStopServingTrafficEdge',
      desc: '',
      args: [],
    );
  }

  /// `Serve traffic too (edge node)`
  String get serverServeTrafficTooEdgeNode {
    return Intl.message(
      'Serve traffic too (edge node)',
      name: 'serverServeTrafficTooEdgeNode',
      desc: '',
      args: [],
    );
  }

  /// `Remove from the cluster`
  String get serverRemoveFromTheCluster2 {
    return Intl.message(
      'Remove from the cluster',
      name: 'serverRemoveFromTheCluster2',
      desc: '',
      args: [],
    );
  }

  /// `Your role does not include viewing the server (server.view).`
  String get serverYourRoleDoesNotInclude {
    return Intl.message(
      'Your role does not include viewing the server (server.view).',
      name: 'serverYourRoleDoesNotInclude',
      desc: '',
      args: [],
    );
  }

  /// `Cluster nodes, their resources and recent warnings.`
  String get serverClusterNodesTheirResourcesAnd {
    return Intl.message(
      'Cluster nodes, their resources and recent warnings.',
      name: 'serverClusterNodesTheirResourcesAnd',
      desc: '',
      args: [],
    );
  }

  /// `Gateway`
  String get serverGateway {
    return Intl.message('Gateway', name: 'serverGateway', desc: '', args: []);
  }

  /// `Add node`
  String get serverAddNode {
    return Intl.message('Add node', name: 'serverAddNode', desc: '', args: []);
  }

  /// `Warnings`
  String get serverWarnings {
    return Intl.message('Warnings', name: 'serverWarnings', desc: '', args: []);
  }

  /// `In`
  String get serverIn {
    return Intl.message('In', name: 'serverIn', desc: '', args: []);
  }

  /// `Out`
  String get serverOut {
    return Intl.message('Out', name: 'serverOut', desc: '', args: []);
  }

  /// `History · {historyName} · last {range}`
  String serverHistoryLast(Object historyName, Object range) {
    return Intl.message(
      'History · $historyName · last $range',
      name: 'serverHistoryLast',
      desc: '',
      args: [historyName, range],
    );
  }

  /// `Volumes`
  String get serverVolumes {
    return Intl.message('Volumes', name: 'serverVolumes', desc: '', args: []);
  }

  /// `No volumes yet. Apps get one under Settings → Volumes; databases have their own.`
  String get serverNoVolumesYetAppsGet {
    return Intl.message(
      'No volumes yet. Apps get one under Settings → Volumes; databases have their own.',
      name: 'serverNoVolumesYetAppsGet',
      desc: '',
      args: [],
    );
  }

  /// `{requestedBytes} · usage shows while the app or database runs`
  String serverUsageShowsWhileTheApp(Object requestedBytes) {
    return Intl.message(
      '$requestedBytes · usage shows while the app or database runs',
      name: 'serverUsageShowsWhileTheApp',
      desc: '',
      args: [requestedBytes],
    );
  }

  /// `On local-path storage (the k3s default) a size is not enforced: a volume can grow past it until the node's disk is full. The "Volume almost full" alert warns at 90 %.`
  String get serverOnLocalPathStorageThe {
    return Intl.message(
      'On local-path storage (the k3s default) a size is not enforced: a volume can grow past it until the node\'s disk is full. The "Volume almost full" alert warns at 90 %.',
      name: 'serverOnLocalPathStorageThe',
      desc: '',
      args: [],
    );
  }

  /// `Your role does not include server.view.`
  String get gatewayYourRoleDoesNotInclude {
    return Intl.message(
      'Your role does not include server.view.',
      name: 'gatewayYourRoleDoesNotInclude',
      desc: '',
      args: [],
    );
  }

  /// `nectar-gateway serves every domain on ports 80/443: routes, certificates, rate limits and blocked IPs.`
  String get gatewayNectarGatewayServesEveryDomain {
    return Intl.message(
      'nectar-gateway serves every domain on ports 80/443: routes, certificates, rate limits and blocked IPs.',
      name: 'gatewayNectarGatewayServesEveryDomain',
      desc: '',
      args: [],
    );
  }

  /// `No routes`
  String get gatewayNoRoutes {
    return Intl.message(
      'No routes',
      name: 'gatewayNoRoutes',
      desc: '',
      args: [],
    );
  }

  /// `Domains of your apps appear here.`
  String get gatewayDomainsOfYourAppsAppear {
    return Intl.message(
      'Domains of your apps appear here.',
      name: 'gatewayDomainsOfYourAppsAppear',
      desc: '',
      args: [],
    );
  }

  /// `Rate limits, timeouts and body limits are set per domain on the app page (Domains → edit).`
  String get gatewayRateLimitsTimeoutsAndBody {
    return Intl.message(
      'Rate limits, timeouts and body limits are set per domain on the app page (Domains → edit).',
      name: 'gatewayRateLimitsTimeoutsAndBody',
      desc: '',
      args: [],
    );
  }

  /// `Let's Encrypt certificates renew automatically about 30 days before they expire; the "Certificate expiring" alert warns when one does not.`
  String get gatewayLetSEncryptCertificatesRenew {
    return Intl.message(
      'Let\'s Encrypt certificates renew automatically about 30 days before they expire; the "Certificate expiring" alert warns when one does not.',
      name: 'gatewayLetSEncryptCertificatesRenew',
      desc: '',
      args: [],
    );
  }

  /// `Wildcard certificates`
  String get gatewayWildcardCertificates {
    return Intl.message(
      'Wildcard certificates',
      name: 'gatewayWildcardCertificates',
      desc: '',
      args: [],
    );
  }

  /// `One certificate for *.your-domain serves every app at once, with no Let's Encrypt order per app. This needs nectar-gateway 1.2.2 or newer: update it under Settings → Updates.`
  String get gatewayOneCertificateForYourDomain {
    return Intl.message(
      'One certificate for *.your-domain serves every app at once, with no Let\'s Encrypt order per app. This needs nectar-gateway 1.2.2 or newer: update it under Settings → Updates.',
      name: 'gatewayOneCertificateForYourDomain',
      desc: '',
      args: [],
    );
  }

  /// `A wildcard certificate (*.example.com) is proved over DNS: the gateway adds a TXT record through your DNS provider. Apps under it get HTTPS the moment they are created.`
  String get gatewayAWildcardCertificateExampleCom {
    return Intl.message(
      'A wildcard certificate (*.example.com) is proved over DNS: the gateway adds a TXT record through your DNS provider. Apps under it get HTTPS the moment they are created.',
      name: 'gatewayAWildcardCertificateExampleCom',
      desc: '',
      args: [],
    );
  }

  /// `The gateway's Let's Encrypt client is off: set the Let's Encrypt email (Settings → Gateway) first.`
  String get gatewayTheGatewaySLetS {
    return Intl.message(
      'The gateway\'s Let\'s Encrypt client is off: set the Let\'s Encrypt email (Settings → Gateway) first.',
      name: 'gatewayTheGatewaySLetS',
      desc: '',
      args: [],
    );
  }

  /// `Remove the DNS provider?`
  String get gatewayRemoveTheDnsProvider {
    return Intl.message(
      'Remove the DNS provider?',
      name: 'gatewayRemoveTheDnsProvider',
      desc: '',
      args: [],
    );
  }

  /// `Wildcard certificates stop renewing until you set it again.`
  String get gatewayWildcardCertificatesStopRenewingUntil {
    return Intl.message(
      'Wildcard certificates stop renewing until you set it again.',
      name: 'gatewayWildcardCertificatesStopRenewingUntil',
      desc: '',
      args: [],
    );
  }

  /// `Get {baseWildcard}`
  String gatewayGet(Object baseWildcard) {
    return Intl.message(
      'Get $baseWildcard',
      name: 'gatewayGet',
      desc: '',
      args: [baseWildcard],
    );
  }

  /// `Other domain…`
  String get gatewayOtherDomain {
    return Intl.message(
      'Other domain…',
      name: 'gatewayOtherDomain',
      desc: '',
      args: [],
    );
  }

  /// `Delete {domain}?`
  String gatewayDelete(Object domain) {
    return Intl.message(
      'Delete $domain?',
      name: 'gatewayDelete',
      desc: '',
      args: [domain],
    );
  }

  /// `Hosts it serves fall back to their own Let's Encrypt certificates (ordered per host).`
  String get gatewayHostsItServesFallBack {
    return Intl.message(
      'Hosts it serves fall back to their own Let\'s Encrypt certificates (ordered per host).',
      name: 'gatewayHostsItServesFallBack',
      desc: '',
      args: [],
    );
  }

  /// `Renew now`
  String get gatewayRenewNow {
    return Intl.message(
      'Renew now',
      name: 'gatewayRenewNow',
      desc: '',
      args: [],
    );
  }

  /// `In Cloudflare: My Profile → API Tokens → Create Token → "Edit zone DNS", limited to your zone (permissions Zone:Read and DNS:Edit). The token is stored in the gateway and never shown again.`
  String get gatewayInCloudflareMyProfileApi {
    return Intl.message(
      'In Cloudflare: My Profile → API Tokens → Create Token → "Edit zone DNS", limited to your zone (permissions Zone:Read and DNS:Edit). The token is stored in the gateway and never shown again.',
      name: 'gatewayInCloudflareMyProfileApi',
      desc: '',
      args: [],
    );
  }

  /// `API token`
  String get gatewayApiToken {
    return Intl.message(
      'API token',
      name: 'gatewayApiToken',
      desc: '',
      args: [],
    );
  }

  /// `Wildcard certificate`
  String get gatewayWildcardCertificate {
    return Intl.message(
      'Wildcard certificate',
      name: 'gatewayWildcardCertificate',
      desc: '',
      args: [],
    );
  }

  /// `Domain`
  String get gatewayDomain {
    return Intl.message('Domain', name: 'gatewayDomain', desc: '', args: []);
  }

  /// `*.example.com`
  String get gatewayExampleCom {
    return Intl.message(
      '*.example.com',
      name: 'gatewayExampleCom',
      desc: '',
      args: [],
    );
  }

  /// `Also the domain itself (example.com)`
  String get gatewayAlsoTheDomainItselfExample {
    return Intl.message(
      'Also the domain itself (example.com)',
      name: 'gatewayAlsoTheDomainItselfExample',
      desc: '',
      args: [],
    );
  }

  /// `Order`
  String get gatewayOrder {
    return Intl.message('Order', name: 'gatewayOrder', desc: '', args: []);
  }

  /// `Rate limits`
  String get gatewayRateLimits {
    return Intl.message(
      'Rate limits',
      name: 'gatewayRateLimits',
      desc: '',
      args: [],
    );
  }

  /// `This nectar-gateway blocks a client over a rate limit for 24 hours. Update it to answer 429 instead.`
  String get gatewayThisNectarGatewayBlocksA {
    return Intl.message(
      'This nectar-gateway blocks a client over a rate limit for 24 hours. Update it to answer 429 instead.',
      name: 'gatewayThisNectarGatewayBlocksA',
      desc: '',
      args: [],
    );
  }

  /// `Above a domain's rate limit a client gets 429 and may retry a second later. A client that keeps going (more than ten times the limit refused within a minute) is blocked:`
  String get gatewayAboveADomainSRate {
    return Intl.message(
      'Above a domain\'s rate limit a client gets 429 and may retry a second later. A client that keeps going (more than ten times the limit refused within a minute) is blocked:',
      name: 'gatewayAboveADomainSRate',
      desc: '',
      args: [],
    );
  }

  /// `Blocked IPs`
  String get gatewayBlockedIps {
    return Intl.message(
      'Blocked IPs',
      name: 'gatewayBlockedIps',
      desc: '',
      args: [],
    );
  }

  /// `Block IP`
  String get gatewayBlockIp {
    return Intl.message('Block IP', name: 'gatewayBlockIp', desc: '', args: []);
  }

  /// `Nobody is blocked. The gateway blocks scanners (requests for .env, wp-login …) by itself.`
  String get gatewayNobodyIsBlockedTheGateway {
    return Intl.message(
      'Nobody is blocked. The gateway blocks scanners (requests for .env, wp-login …) by itself.',
      name: 'gatewayNobodyIsBlockedTheGateway',
      desc: '',
      args: [],
    );
  }

  /// `Unblock`
  String get gatewayUnblock {
    return Intl.message('Unblock', name: 'gatewayUnblock', desc: '', args: []);
  }

  /// `Block an IP`
  String get gatewayBlockAnIp {
    return Intl.message(
      'Block an IP',
      name: 'gatewayBlockAnIp',
      desc: '',
      args: [],
    );
  }

  /// `IP address`
  String get gatewayIpAddress {
    return Intl.message(
      'IP address',
      name: 'gatewayIpAddress',
      desc: '',
      args: [],
    );
  }

  /// `Reason (optional)`
  String get gatewayReasonOptional {
    return Intl.message(
      'Reason (optional)',
      name: 'gatewayReasonOptional',
      desc: '',
      args: [],
    );
  }

  /// `For`
  String get gatewayFor {
    return Intl.message('For', name: 'gatewayFor', desc: '', args: []);
  }

  /// `1 hour`
  String get gateway1Hour {
    return Intl.message('1 hour', name: 'gateway1Hour', desc: '', args: []);
  }

  /// `1 day`
  String get gateway1Day {
    return Intl.message('1 day', name: 'gateway1Day', desc: '', args: []);
  }

  /// `1 week`
  String get gateway1Week {
    return Intl.message('1 week', name: 'gateway1Week', desc: '', args: []);
  }

  /// `until unblocked`
  String get gatewayUntilUnblocked {
    return Intl.message(
      'until unblocked',
      name: 'gatewayUntilUnblocked',
      desc: '',
      args: [],
    );
  }

  /// `Block`
  String get gatewayBlock {
    return Intl.message('Block', name: 'gatewayBlock', desc: '', args: []);
  }

  /// `Latest requests`
  String get gatewayLatestRequests {
    return Intl.message(
      'Latest requests',
      name: 'gatewayLatestRequests',
      desc: '',
      args: [],
    );
  }

  /// `All domains`
  String get gatewayAllDomains {
    return Intl.message(
      'All domains',
      name: 'gatewayAllDomains',
      desc: '',
      args: [],
    );
  }

  /// `Status`
  String get gatewayStatus {
    return Intl.message('Status', name: 'gatewayStatus', desc: '', args: []);
  }

  /// `Any`
  String get gatewayAny {
    return Intl.message('Any', name: 'gatewayAny', desc: '', args: []);
  }

  /// `Client IP`
  String get gatewayClientIp {
    return Intl.message(
      'Client IP',
      name: 'gatewayClientIp',
      desc: '',
      args: [],
    );
  }

  /// `No requests match. The gateway keeps the latest requests in memory (ACCESS_LOG_SIZE, 2000 by default).`
  String get gatewayNoRequestsMatchTheGateway {
    return Intl.message(
      'No requests match. The gateway keeps the latest requests in memory (ACCESS_LOG_SIZE, 2000 by default).',
      name: 'gatewayNoRequestsMatchTheGateway',
      desc: '',
      args: [],
    );
  }

  /// `{ms} ms`
  String gatewayMs(Object ms) {
    return Intl.message('$ms ms', name: 'gatewayMs', desc: '', args: [ms]);
  }

  /// `Block {ip}`
  String gatewayBlock2(Object ip) {
    return Intl.message(
      'Block $ip',
      name: 'gatewayBlock2',
      desc: '',
      args: [ip],
    );
  }

  /// `Back`
  String get projectBack {
    return Intl.message('Back', name: 'projectBack', desc: '', args: []);
  }

  /// `Open the app`
  String get projectOpenTheApp {
    return Intl.message(
      'Open the app',
      name: 'projectOpenTheApp',
      desc: '',
      args: [],
    );
  }

  /// `The app slug; its default domain is <name>-<project>.<base domain>`
  String get projectTheAppSlugItsDefault {
    return Intl.message(
      'The app slug; its default domain is <name>-<project>.<base domain>',
      name: 'projectTheAppSlugItsDefault',
      desc: '',
      args: [],
    );
  }

  /// `Passwords are generated and stored as secret variables. The app is deployed right away.`
  String get projectPasswordsAreGeneratedAndStored {
    return Intl.message(
      'Passwords are generated and stored as secret variables. The app is deployed right away.',
      name: 'projectPasswordsAreGeneratedAndStored',
      desc: '',
      args: [],
    );
  }

  /// `Import {what}`
  String projectImport(Object what) {
    return Intl.message(
      'Import $what',
      name: 'projectImport',
      desc: '',
      args: [what],
    );
  }

  /// `Choose {what}`
  String projectChoose(Object what) {
    return Intl.message(
      'Choose $what',
      name: 'projectChoose',
      desc: '',
      args: [what],
    );
  }

  /// `or paste it below`
  String get projectOrPasteItBelow {
    return Intl.message(
      'or paste it below',
      name: 'projectOrPasteItBelow',
      desc: '',
      args: [],
    );
  }

  /// `Deploy the apps`
  String get projectDeployTheApps {
    return Intl.message(
      'Deploy the apps',
      name: 'projectDeployTheApps',
      desc: '',
      args: [],
    );
  }

  /// `Copied — apply it with: nectar apply -f nectar.yaml --project {slug}`
  String projectCopiedApplyItWithNectar(Object slug) {
    return Intl.message(
      'Copied — apply it with: nectar apply -f nectar.yaml --project $slug',
      name: 'projectCopiedApplyItWithNectar',
      desc: '',
      args: [slug],
    );
  }

  /// `Env groups`
  String get projectEnvGroups {
    return Intl.message(
      'Env groups',
      name: 'projectEnvGroups',
      desc: '',
      args: [],
    );
  }

  /// `New group`
  String get projectNewGroup {
    return Intl.message(
      'New group',
      name: 'projectNewGroup',
      desc: '',
      args: [],
    );
  }

  /// `Variables several apps share (a region, an API endpoint, a Sentry DSN). Apps choose their groups on the Environment tab.`
  String get projectVariablesSeveralAppsShareA {
    return Intl.message(
      'Variables several apps share (a region, an API endpoint, a Sentry DSN). Apps choose their groups on the Environment tab.',
      name: 'projectVariablesSeveralAppsShareA',
      desc: '',
      args: [],
    );
  }

  /// `Invalid variable name "{key}": use letters, digits and _`
  String projectInvalidVariableNameUseLetters(Object key) {
    return Intl.message(
      'Invalid variable name "$key": use letters, digits and _',
      name: 'projectInvalidVariableNameUseLetters',
      desc: '',
      args: [key],
    );
  }

  /// `Saved`
  String get projectSaved {
    return Intl.message('Saved', name: 'projectSaved', desc: '', args: []);
  }

  /// `Delete the group {gName}?`
  String projectDeleteTheGroup(Object gName) {
    return Intl.message(
      'Delete the group $gName?',
      name: 'projectDeleteTheGroup',
      desc: '',
      args: [gName],
    );
  }

  /// `Its variables are deleted.`
  String get projectItsVariablesAreDeleted {
    return Intl.message(
      'Its variables are deleted.',
      name: 'projectItsVariablesAreDeleted',
      desc: '',
      args: [],
    );
  }

  /// `Show secret values`
  String get projectShowSecretValues {
    return Intl.message(
      'Show secret values',
      name: 'projectShowSecretValues',
      desc: '',
      args: [],
    );
  }

  /// `Shared env groups`
  String get projectSharedEnvGroups {
    return Intl.message(
      'Shared env groups',
      name: 'projectSharedEnvGroups',
      desc: '',
      args: [],
    );
  }

  /// `This project has no env groups yet (project page → Env groups).`
  String get projectThisProjectHasNoEnv {
    return Intl.message(
      'This project has no env groups yet (project page → Env groups).',
      name: 'projectThisProjectHasNoEnv',
      desc: '',
      args: [],
    );
  }

  /// `The app gets every variable of the chosen groups; its own variables win.`
  String get projectTheAppGetsEveryVariable {
    return Intl.message(
      'The app gets every variable of the chosen groups; its own variables win.',
      name: 'projectTheAppGetsEveryVariable',
      desc: '',
      args: [],
    );
  }

  /// `Copy {appName}`
  String projectCopy(Object appName) {
    return Intl.message(
      'Copy $appName',
      name: 'projectCopy',
      desc: '',
      args: [appName],
    );
  }

  /// `Into project`
  String get projectIntoProject {
    return Intl.message(
      'Into project',
      name: 'projectIntoProject',
      desc: '',
      args: [],
    );
  }

  /// `Slug`
  String get projectSlug {
    return Intl.message('Slug', name: 'projectSlug', desc: '', args: []);
  }

  /// `Copies the settings, variables (secrets too), config files and env groups. Volumes start empty; nothing is deployed.`
  String get projectCopiesTheSettingsVariablesSecrets {
    return Intl.message(
      'Copies the settings, variables (secrets too), config files and env groups. Volumes start empty; nothing is deployed.',
      name: 'projectCopiesTheSettingsVariablesSecrets',
      desc: '',
      args: [],
    );
  }

  /// `New environment`
  String get projectNewEnvironment {
    return Intl.message(
      'New environment',
      name: 'projectNewEnvironment',
      desc: '',
      args: [],
    );
  }

  /// `Environment of the copy`
  String get projectEnvironmentOfTheCopy {
    return Intl.message(
      'Environment of the copy',
      name: 'projectEnvironmentOfTheCopy',
      desc: '',
      args: [],
    );
  }

  /// `Project slug of the copy`
  String get projectProjectSlugOfTheCopy {
    return Intl.message(
      'Project slug of the copy',
      name: 'projectProjectSlugOfTheCopy',
      desc: '',
      args: [],
    );
  }

  /// `Environment of {slug}`
  String projectEnvironmentOf(Object slug) {
    return Intl.message(
      'Environment of $slug',
      name: 'projectEnvironmentOf',
      desc: '',
      args: [slug],
    );
  }

  /// `Copies the env groups, the databases (new and empty, with new passwords) and the apps with their settings, variables and config files. Nothing is deployed. Later, "Promote" deploys the live build of an app to the same app in the other environment.`
  String get projectCopiesTheEnvGroupsThe {
    return Intl.message(
      'Copies the env groups, the databases (new and empty, with new passwords) and the apps with their settings, variables and config files. Nothing is deployed. Later, "Promote" deploys the live build of an app to the same app in the other environment.',
      name: 'projectCopiesTheEnvGroupsThe',
      desc: '',
      args: [],
    );
  }

  /// `Promote {appName} to {target}?`
  String projectPromoteTo(Object appName, Object target) {
    return Intl.message(
      'Promote $appName to $target?',
      name: 'projectPromoteTo',
      desc: '',
      args: [appName, target],
    );
  }

  /// `The live build of {slug} (its image or uploaded file) is deployed to {slug2}/{slug3}. Its settings and variables there stay as they are.`
  String projectTheLiveBuildOfIts(Object slug, Object slug2, Object slug3) {
    return Intl.message(
      'The live build of $slug (its image or uploaded file) is deployed to $slug2/$slug3. Its settings and variables there stay as they are.',
      name: 'projectTheLiveBuildOfIts',
      desc: '',
      args: [slug, slug2, slug3],
    );
  }

  /// `Promote`
  String get projectPromote {
    return Intl.message('Promote', name: 'projectPromote', desc: '', args: []);
  }

  /// `New project`
  String get projectNewProject {
    return Intl.message(
      'New project',
      name: 'projectNewProject',
      desc: '',
      args: [],
    );
  }

  /// `My Shop`
  String get projectMyShop {
    return Intl.message('My Shop', name: 'projectMyShop', desc: '', args: []);
  }

  /// `Create`
  String get projectCreate {
    return Intl.message('Create', name: 'projectCreate', desc: '', args: []);
  }

  /// `All projects`
  String get projectAllProjects {
    return Intl.message(
      'All projects',
      name: 'projectAllProjects',
      desc: '',
      args: [],
    );
  }

  /// `Project`
  String get projectProject {
    return Intl.message('Project', name: 'projectProject', desc: '', args: []);
  }

  /// `Import / export`
  String get projectImportExport {
    return Intl.message(
      'Import / export',
      name: 'projectImportExport',
      desc: '',
      args: [],
    );
  }

  /// `Import docker-compose.yml`
  String get projectImportDockerComposeYml {
    return Intl.message(
      'Import docker-compose.yml',
      name: 'projectImportDockerComposeYml',
      desc: '',
      args: [],
    );
  }

  /// `Apply nectar.yaml`
  String get projectApplyNectarYaml {
    return Intl.message(
      'Apply nectar.yaml',
      name: 'projectApplyNectarYaml',
      desc: '',
      args: [],
    );
  }

  /// `Export nectar.yaml`
  String get projectExportNectarYaml {
    return Intl.message(
      'Export nectar.yaml',
      name: 'projectExportNectarYaml',
      desc: '',
      args: [],
    );
  }

  /// `New environment (copy)…`
  String get projectNewEnvironmentCopy {
    return Intl.message(
      'New environment (copy)…',
      name: 'projectNewEnvironmentCopy',
      desc: '',
      args: [],
    );
  }

  /// `Activity`
  String get projectActivity {
    return Intl.message(
      'Activity',
      name: 'projectActivity',
      desc: '',
      args: [],
    );
  }

  /// `No apps in this project yet`
  String get projectNoAppsInThisProject {
    return Intl.message(
      'No apps in this project yet',
      name: 'projectNoAppsInThisProject',
      desc: '',
      args: [],
    );
  }

  /// `Deploy a JAR, a binary, a static site, Node, Python, PHP or any Docker image.`
  String get projectDeployAJarABinary {
    return Intl.message(
      'Deploy a JAR, a binary, a static site, Node, Python, PHP or any Docker image.',
      name: 'projectDeployAJarABinary',
      desc: '',
      args: [],
    );
  }

  /// `Resources`
  String get projectResources {
    return Intl.message(
      'Resources',
      name: 'projectResources',
      desc: '',
      args: [],
    );
  }

  /// `CPU requested`
  String get projectCpuRequested {
    return Intl.message(
      'CPU requested',
      name: 'projectCpuRequested',
      desc: '',
      args: [],
    );
  }

  /// `Memory requested`
  String get projectMemoryRequested {
    return Intl.message(
      'Memory requested',
      name: 'projectMemoryRequested',
      desc: '',
      args: [],
    );
  }

  /// `Apps`
  String get projectApps {
    return Intl.message('Apps', name: 'projectApps', desc: '', args: []);
  }

  /// `Requests of all replicas. Live usage arrives with the metrics view.`
  String get projectRequestsOfAllReplicasLive {
    return Intl.message(
      'Requests of all replicas. Live usage arrives with the metrics view.',
      name: 'projectRequestsOfAllReplicasLive',
      desc: '',
      args: [],
    );
  }

  /// `Each project has its own Kubernetes namespace and optional quota.`
  String get projectEachProjectHasItsOwn {
    return Intl.message(
      'Each project has its own Kubernetes namespace and optional quota.',
      name: 'projectEachProjectHasItsOwn',
      desc: '',
      args: [],
    );
  }

  /// `No projects yet`
  String get projectNoProjectsYet {
    return Intl.message(
      'No projects yet',
      name: 'projectNoProjectsYet',
      desc: '',
      args: [],
    );
  }

  /// `Search projects`
  String get projectSearchProjects {
    return Intl.message(
      'Search projects',
      name: 'projectSearchProjects',
      desc: '',
      args: [],
    );
  }

  /// `Adopted from an existing namespace: other workloads there are left alone.`
  String get projectAdoptedFromAnExistingNamespace {
    return Intl.message(
      'Adopted from an existing namespace: other workloads there are left alone.',
      name: 'projectAdoptedFromAnExistingNamespace',
      desc: '',
      args: [],
    );
  }

  /// `Adopted`
  String get projectAdopted {
    return Intl.message('Adopted', name: 'projectAdopted', desc: '', args: []);
  }

  /// `Connect to your server`
  String get authConnectToYourServer {
    return Intl.message(
      'Connect to your server',
      name: 'authConnectToYourServer',
      desc: '',
      args: [],
    );
  }

  /// `Nectar Apps runs on your own server. Enter the address of its panel.`
  String get authNectarAppsRunsOnYour {
    return Intl.message(
      'Nectar Apps runs on your own server. Enter the address of its panel.',
      name: 'authNectarAppsRunsOnYour',
      desc: '',
      args: [],
    );
  }

  /// `Panel address`
  String get authPanelAddress {
    return Intl.message(
      'Panel address',
      name: 'authPanelAddress',
      desc: '',
      args: [],
    );
  }

  /// `Before the panel has a domain, open an SSH tunnel and use http://localhost:8480.`
  String get authBeforeThePanelHasA {
    return Intl.message(
      'Before the panel has a domain, open an SSH tunnel and use http://localhost:8480.',
      name: 'authBeforeThePanelHasA',
      desc: '',
      args: [],
    );
  }

  /// `Connect`
  String get authConnect {
    return Intl.message('Connect', name: 'authConnect', desc: '', args: []);
  }

  /// `Reset your password`
  String get authResetYourPassword {
    return Intl.message(
      'Reset your password',
      name: 'authResetYourPassword',
      desc: '',
      args: [],
    );
  }

  /// `We email you a link to choose a new password (if the panel can send emails).`
  String get authWeEmailYouALink {
    return Intl.message(
      'We email you a link to choose a new password (if the panel can send emails).',
      name: 'authWeEmailYouALink',
      desc: '',
      args: [],
    );
  }

  /// `Send link`
  String get authSendLink {
    return Intl.message('Send link', name: 'authSendLink', desc: '', args: []);
  }

  /// `If the address has an account, a reset link is on its way.`
  String get authIfTheAddressHasAn {
    return Intl.message(
      'If the address has an account, a reset link is on its way.',
      name: 'authIfTheAddressHasAn',
      desc: '',
      args: [],
    );
  }

  /// `Cannot reach the panel`
  String get authCannotReachThePanel {
    return Intl.message(
      'Cannot reach the panel',
      name: 'authCannotReachThePanel',
      desc: '',
      args: [],
    );
  }

  /// `Use another server`
  String get authUseAnotherServer {
    return Intl.message(
      'Use another server',
      name: 'authUseAnotherServer',
      desc: '',
      args: [],
    );
  }

  /// `Enter the 6-digit code from your authenticator app, or one of your recovery codes.`
  String get authEnterThe6DigitCode {
    return Intl.message(
      'Enter the 6-digit code from your authenticator app, or one of your recovery codes.',
      name: 'authEnterThe6DigitCode',
      desc: '',
      args: [],
    );
  }

  /// `Code`
  String get authCode {
    return Intl.message('Code', name: 'authCode', desc: '', args: []);
  }

  /// `Verify`
  String get authVerify {
    return Intl.message('Verify', name: 'authVerify', desc: '', args: []);
  }

  /// `Forgot your password?`
  String get authForgotYourPassword {
    return Intl.message(
      'Forgot your password?',
      name: 'authForgotYourPassword',
      desc: '',
      args: [],
    );
  }

  /// `Continue with {pName}`
  String authContinueWith(Object pName) {
    return Intl.message(
      'Continue with $pName',
      name: 'authContinueWith',
      desc: '',
      args: [pName],
    );
  }

  /// `Choose a password`
  String get authChooseAPassword {
    return Intl.message(
      'Choose a password',
      name: 'authChooseAPassword',
      desc: '',
      args: [],
    );
  }

  /// `Password saved`
  String get authPasswordSaved {
    return Intl.message(
      'Password saved',
      name: 'authPasswordSaved',
      desc: '',
      args: [],
    );
  }

  /// `Sign in with your new password and your authenticator code.`
  String get authSignInWithYourNew {
    return Intl.message(
      'Sign in with your new password and your authenticator code.',
      name: 'authSignInWithYourNew',
      desc: '',
      args: [],
    );
  }

  /// `Link expired`
  String get authLinkExpired {
    return Intl.message(
      'Link expired',
      name: 'authLinkExpired',
      desc: '',
      args: [],
    );
  }

  /// `Go to sign-in`
  String get authGoToSignIn {
    return Intl.message(
      'Go to sign-in',
      name: 'authGoToSignIn',
      desc: '',
      args: [],
    );
  }

  /// `At least 10 characters`
  String get authAtLeast10Characters {
    return Intl.message(
      'At least 10 characters',
      name: 'authAtLeast10Characters',
      desc: '',
      args: [],
    );
  }

  /// `Repeat the password`
  String get authRepeatThePassword {
    return Intl.message(
      'Repeat the password',
      name: 'authRepeatThePassword',
      desc: '',
      args: [],
    );
  }

  /// `Welcome to Nectar Apps`
  String get authWelcomeToNectarApps {
    return Intl.message(
      'Welcome to Nectar Apps',
      name: 'authWelcomeToNectarApps',
      desc: '',
      args: [],
    );
  }

  /// `Setup token`
  String get authSetupToken {
    return Intl.message(
      'Setup token',
      name: 'authSetupToken',
      desc: '',
      args: [],
    );
  }

  /// `The installer printed it together with the setup link.`
  String get authTheInstallerPrintedItTogether {
    return Intl.message(
      'The installer printed it together with the setup link.',
      name: 'authTheInstallerPrintedItTogether',
      desc: '',
      args: [],
    );
  }

  /// `Your name`
  String get authYourName {
    return Intl.message('Your name', name: 'authYourName', desc: '', args: []);
  }

  /// `Repeat password`
  String get authRepeatPassword {
    return Intl.message(
      'Repeat password',
      name: 'authRepeatPassword',
      desc: '',
      args: [],
    );
  }

  /// `Create Owner account`
  String get authCreateOwnerAccount {
    return Intl.message(
      'Create Owner account',
      name: 'authCreateOwnerAccount',
      desc: '',
      args: [],
    );
  }

  /// `Requested`
  String get observeRequested {
    return Intl.message(
      'Requested',
      name: 'observeRequested',
      desc: '',
      args: [],
    );
  }

  /// `Totals across {replicas} replicas; the dashed line is what all of them requested together.`
  String observeTotalsAcrossReplicasTheDashed(Object replicas) {
    return Intl.message(
      'Totals across $replicas replicas; the dashed line is what all of them requested together.',
      name: 'observeTotalsAcrossReplicasTheDashed',
      desc: '',
      args: [replicas],
    );
  }

  /// `Traffic · last hour`
  String get observeTrafficLastHour {
    return Intl.message(
      'Traffic · last hour',
      name: 'observeTrafficLastHour',
      desc: '',
      args: [],
    );
  }

  /// `No requests yet. Traffic appears once the app has a domain and the gateway routes requests to it.`
  String get observeNoRequestsYetTrafficAppears {
    return Intl.message(
      'No requests yet. Traffic appears once the app has a domain and the gateway routes requests to it.',
      name: 'observeNoRequestsYetTrafficAppears',
      desc: '',
      args: [],
    );
  }

  /// `Requests`
  String get observeRequests {
    return Intl.message(
      'Requests',
      name: 'observeRequests',
      desc: '',
      args: [],
    );
  }

  /// `Per second`
  String get observePerSecond {
    return Intl.message(
      'Per second',
      name: 'observePerSecond',
      desc: '',
      args: [],
    );
  }

  /// `Errors (5xx)`
  String get observeErrors5xx {
    return Intl.message(
      'Errors (5xx)',
      name: 'observeErrors5xx',
      desc: '',
      args: [],
    );
  }

  /// `No requests yet.`
  String get observeNoRequestsYet {
    return Intl.message(
      'No requests yet.',
      name: 'observeNoRequestsYet',
      desc: '',
      args: [],
    );
  }

  /// `Live`
  String get observeLive {
    return Intl.message('Live', name: 'observeLive', desc: '', args: []);
  }

  /// `History`
  String get observeHistory {
    return Intl.message('History', name: 'observeHistory', desc: '', args: []);
  }

  /// `.* regex`
  String get observeRegex {
    return Intl.message('.* regex', name: 'observeRegex', desc: '', args: []);
  }

  /// `Last hour`
  String get observeLastHour {
    return Intl.message(
      'Last hour',
      name: 'observeLastHour',
      desc: '',
      args: [],
    );
  }

  /// `Last 6 hours`
  String get observeLast6Hours {
    return Intl.message(
      'Last 6 hours',
      name: 'observeLast6Hours',
      desc: '',
      args: [],
    );
  }

  /// `Last 24 hours`
  String get observeLast24Hours {
    return Intl.message(
      'Last 24 hours',
      name: 'observeLast24Hours',
      desc: '',
      args: [],
    );
  }

  /// `Last 7 days`
  String get observeLast7Days {
    return Intl.message(
      'Last 7 days',
      name: 'observeLast7Days',
      desc: '',
      args: [],
    );
  }

  /// `Everything kept`
  String get observeEverythingKept {
    return Intl.message(
      'Everything kept',
      name: 'observeEverythingKept',
      desc: '',
      args: [],
    );
  }

  /// `Search`
  String get observeSearch {
    return Intl.message('Search', name: 'observeSearch', desc: '', args: []);
  }

  /// `Copy shown lines`
  String get observeCopyShownLines {
    return Intl.message(
      'Copy shown lines',
      name: 'observeCopyShownLines',
      desc: '',
      args: [],
    );
  }

  /// `Copied {linesLength} lines`
  String observeCopiedLines(Object linesLength) {
    return Intl.message(
      'Copied $linesLength lines',
      name: 'observeCopiedLines',
      desc: '',
      args: [linesLength],
    );
  }

  /// `Newest first. Logs are kept on the panel for the days set under Server → Disk → Retention.`
  String get observeNewestFirstLogsAreKept {
    return Intl.message(
      'Newest first. Logs are kept on the panel for the days set under Server → Disk → Retention.',
      name: 'observeNewestFirstLogsAreKept',
      desc: '',
      args: [],
    );
  }

  /// `All pods`
  String get observeAllPods {
    return Intl.message('All pods', name: 'observeAllPods', desc: '', args: []);
  }

  /// `Filter`
  String get observeFilter {
    return Intl.message('Filter', name: 'observeFilter', desc: '', args: []);
  }

  /// `Copied {visibleLength} lines`
  String observeCopiedLines2(Object visibleLength) {
    return Intl.message(
      'Copied $visibleLength lines',
      name: 'observeCopiedLines2',
      desc: '',
      args: [visibleLength],
    );
  }

  /// `Download (last 5000 lines)`
  String get observeDownloadLast5000Lines {
    return Intl.message(
      'Download (last 5000 lines)',
      name: 'observeDownloadLast5000Lines',
      desc: '',
      args: [],
    );
  }

  /// `Clear`
  String get observeClear {
    return Intl.message('Clear', name: 'observeClear', desc: '', args: []);
  }

  /// `Before the last restart · {pod}`
  String observeBeforeTheLastRestart(Object pod) {
    return Intl.message(
      'Before the last restart · $pod',
      name: 'observeBeforeTheLastRestart',
      desc: '',
      args: [pod],
    );
  }

  /// `The previous container wrote nothing.`
  String get observeThePreviousContainerWroteNothing {
    return Intl.message(
      'The previous container wrote nothing.',
      name: 'observeThePreviousContainerWroteNothing',
      desc: '',
      args: [],
    );
  }

  /// `Update started`
  String get updateUpdateStarted {
    return Intl.message(
      'Update started',
      name: 'updateUpdateStarted',
      desc: '',
      args: [],
    );
  }

  /// `Update {component}`
  String updateUpdate(Object component) {
    return Intl.message(
      'Update $component',
      name: 'updateUpdate',
      desc: '',
      args: [component],
    );
  }

  /// `Version`
  String get updateVersion {
    return Intl.message('Version', name: 'updateVersion', desc: '', args: []);
  }

  /// `Each step: the database is backed up, the image is switched, and the panel waits until the new version is healthy (and the gateway serves its canary and keeps every route). If a check fails it rolls back on its own.`
  String get updateEachStepTheDatabaseIs {
    return Intl.message(
      'Each step: the database is backed up, the image is switched, and the panel waits until the new version is healthy (and the gateway serves its canary and keeps every route). If a check fails it rolls back on its own.',
      name: 'updateEachStepTheDatabaseIs',
      desc: '',
      args: [],
    );
  }

  /// `Update settings saved`
  String get updateUpdateSettingsSaved {
    return Intl.message(
      'Update settings saved',
      name: 'updateUpdateSettingsSaved',
      desc: '',
      args: [],
    );
  }

  /// `Policy`
  String get updatePolicy {
    return Intl.message('Policy', name: 'updatePolicy', desc: '', args: []);
  }

  /// `Stable`
  String get updateStable {
    return Intl.message('Stable', name: 'updateStable', desc: '', args: []);
  }

  /// `Beta`
  String get updateBeta {
    return Intl.message('Beta', name: 'updateBeta', desc: '', args: []);
  }

  /// `Channel`
  String get updateChannel {
    return Intl.message('Channel', name: 'updateChannel', desc: '', args: []);
  }

  /// `stable, latest, v1.31`
  String get updateStableLatestV131 {
    return Intl.message(
      'stable, latest, v1.31',
      name: 'updateStableLatestV131',
      desc: '',
      args: [],
    );
  }

  /// `Automatic updates`
  String get updateAutomaticUpdates {
    return Intl.message(
      'Automatic updates',
      name: 'updateAutomaticUpdates',
      desc: '',
      args: [],
    );
  }

  /// `Major and breaking releases are never installed automatically. A release that failed its checks is not retried.`
  String get updateMajorAndBreakingReleasesAre {
    return Intl.message(
      'Major and breaking releases are never installed automatically. A release that failed its checks is not retried.',
      name: 'updateMajorAndBreakingReleasesAre',
      desc: '',
      args: [],
    );
  }

  /// `From`
  String get updateFrom {
    return Intl.message('From', name: 'updateFrom', desc: '', args: []);
  }

  /// `Time zone`
  String get updateTimeZone {
    return Intl.message(
      'Time zone',
      name: 'updateTimeZone',
      desc: '',
      args: [],
    );
  }

  /// `UTC, Europe/Kyiv`
  String get updateUtcEuropeKyiv {
    return Intl.message(
      'UTC, Europe/Kyiv',
      name: 'updateUtcEuropeKyiv',
      desc: '',
      args: [],
    );
  }

  /// `Check every`
  String get updateCheckEvery {
    return Intl.message(
      'Check every',
      name: 'updateCheckEvery',
      desc: '',
      args: [],
    );
  }

  /// `No days and no times = any time. A window like 22:00–02:00 crosses midnight. k3s upgrades use the same window.`
  String get updateNoDaysAndNoTimes {
    return Intl.message(
      'No days and no times = any time. A window like 22:00–02:00 crosses midnight. k3s upgrades use the same window.',
      name: 'updateNoDaysAndNoTimes',
      desc: '',
      args: [],
    );
  }

  /// `Update sources`
  String get updateUpdateSources {
    return Intl.message(
      'Update sources',
      name: 'updateUpdateSources',
      desc: '',
      args: [],
    );
  }

  /// `Panel feed URL`
  String get updatePanelFeedUrl {
    return Intl.message(
      'Panel feed URL',
      name: 'updatePanelFeedUrl',
      desc: '',
      args: [],
    );
  }

  /// `Gateway feed URL`
  String get updateGatewayFeedUrl {
    return Intl.message(
      'Gateway feed URL',
      name: 'updateGatewayFeedUrl',
      desc: '',
      args: [],
    );
  }

  /// `Empty = the official feeds. When a feed cannot be read, the panel asks the GitHub Releases API instead.`
  String get updateEmptyTheOfficialFeedsWhen {
    return Intl.message(
      'Empty = the official feeds. When a feed cannot be read, the panel asks the GitHub Releases API instead.',
      name: 'updateEmptyTheOfficialFeedsWhen',
      desc: '',
      args: [],
    );
  }

  /// `Checked for updates`
  String get updateCheckedForUpdates {
    return Intl.message(
      'Checked for updates',
      name: 'updateCheckedForUpdates',
      desc: '',
      args: [],
    );
  }

  /// `Updates`
  String get updateUpdates {
    return Intl.message('Updates', name: 'updateUpdates', desc: '', args: []);
  }

  /// `The panel, nectar-gateway and k3s. Every update is backed up first, checked after the switch and rolled back when a check fails.`
  String get updateThePanelNectarGatewayAnd {
    return Intl.message(
      'The panel, nectar-gateway and k3s. Every update is backed up first, checked after the switch and rolled back when a check fails.',
      name: 'updateThePanelNectarGatewayAnd',
      desc: '',
      args: [],
    );
  }

  /// `Check now`
  String get updateCheckNow {
    return Intl.message(
      'Check now',
      name: 'updateCheckNow',
      desc: '',
      args: [],
    );
  }

  /// `Versions`
  String get updateVersions {
    return Intl.message('Versions', name: 'updateVersions', desc: '', args: []);
  }

  /// `Allow again`
  String get updateAllowAgain {
    return Intl.message(
      'Allow again',
      name: 'updateAllowAgain',
      desc: '',
      args: [],
    );
  }

  /// `Update k3s to {v}?`
  String updateUpdateK3sTo(Object v) {
    return Intl.message(
      'Update k3s to $v?',
      name: 'updateUpdateK3sTo',
      desc: '',
      args: [v],
    );
  }

  /// `The system-upgrade-controller upgrades one node at a time: each node is cordoned, k3s restarts with the new version and the node is uncordoned. Pods on a node restart during its upgrade; on a single-node server apps are briefly unavailable.`
  String get updateTheSystemUpgradeControllerUpgrades {
    return Intl.message(
      'The system-upgrade-controller upgrades one node at a time: each node is cordoned, k3s restarts with the new version and the node is uncordoned. Pods on a node restart during its upgrade; on a single-node server apps are briefly unavailable.',
      name: 'updateTheSystemUpgradeControllerUpgrades',
      desc: '',
      args: [],
    );
  }

  /// `Upgrade`
  String get updateUpgrade {
    return Intl.message('Upgrade', name: 'updateUpgrade', desc: '', args: []);
  }

  /// `k3s upgrade to {v} started: node versions update here as it progresses`
  String updateK3sUpgradeToStartedNode(Object v) {
    return Intl.message(
      'k3s upgrade to $v started: node versions update here as it progresses',
      name: 'updateK3sUpgradeToStartedNode',
      desc: '',
      args: [v],
    );
  }

  /// `Updating k3s from here needs the system-upgrade-controller: re-run install.sh with --k3s-upgrades.`
  String get updateUpdatingK3sFromHereNeeds {
    return Intl.message(
      'Updating k3s from here needs the system-upgrade-controller: re-run install.sh with --k3s-upgrades.',
      name: 'updateUpdatingK3sFromHereNeeds',
      desc: '',
      args: [],
    );
  }

  /// `Update…`
  String get updateUpdate2 {
    return Intl.message('Update…', name: 'updateUpdate2', desc: '', args: []);
  }

  /// `Updating · {summary}`
  String updateUpdating(Object summary) {
    return Intl.message(
      'Updating · $summary',
      name: 'updateUpdating',
      desc: '',
      args: [summary],
    );
  }

  /// `The panel restarts now: this page reconnects by itself in a few seconds.`
  String get updateThePanelRestartsNowThis {
    return Intl.message(
      'The panel restarts now: this page reconnects by itself in a few seconds.',
      name: 'updateThePanelRestartsNowThis',
      desc: '',
      args: [],
    );
  }

  /// `Roll back {summary}?`
  String updateRollBack(Object summary) {
    return Intl.message(
      'Roll back $summary?',
      name: 'updateRollBack',
      desc: '',
      args: [summary],
    );
  }

  /// `Roll back`
  String get updateRollBack2 {
    return Intl.message(
      'Roll back',
      name: 'updateRollBack2',
      desc: '',
      args: [],
    );
  }

  /// `No update has run on this server yet.`
  String get updateNoUpdateHasRunOn {
    return Intl.message(
      'No update has run on this server yet.',
      name: 'updateNoUpdateHasRunOn',
      desc: '',
      args: [],
    );
  }

  /// `Members`
  String get acceMembers {
    return Intl.message('Members', name: 'acceMembers', desc: '', args: []);
  }

  /// `Add member`
  String get acceAddMember {
    return Intl.message(
      'Add member',
      name: 'acceAddMember',
      desc: '',
      args: [],
    );
  }

  /// `User`
  String get acceUser {
    return Intl.message('User', name: 'acceUser', desc: '', args: []);
  }

  /// `You can only give roles whose permissions you hold in this project.`
  String get acceYouCanOnlyGiveRoles {
    return Intl.message(
      'You can only give roles whose permissions you hold in this project.',
      name: 'acceYouCanOnlyGiveRoles',
      desc: '',
      args: [],
    );
  }

  /// `Remove from project`
  String get acceRemoveFromProject {
    return Intl.message(
      'Remove from project',
      name: 'acceRemoveFromProject',
      desc: '',
      args: [],
    );
  }

  /// `Roles`
  String get acceRoles {
    return Intl.message('Roles', name: 'acceRoles', desc: '', args: []);
  }

  /// `A role is a set of permissions. Users get roles everywhere or for one project; API tokens can be limited to a role.`
  String get acceARoleIsASet {
    return Intl.message(
      'A role is a set of permissions. Users get roles everywhere or for one project; API tokens can be limited to a role.',
      name: 'acceARoleIsASet',
      desc: '',
      args: [],
    );
  }

  /// `New role`
  String get acceNewRole {
    return Intl.message('New role', name: 'acceNewRole', desc: '', args: []);
  }

  /// `Changing roles needs role.manage.`
  String get acceChangingRolesNeedsRoleManage {
    return Intl.message(
      'Changing roles needs role.manage.',
      name: 'acceChangingRolesNeedsRoleManage',
      desc: '',
      args: [],
    );
  }

  /// `Delete role {rName}?`
  String acceDeleteRole(Object rName) {
    return Intl.message(
      'Delete role $rName?',
      name: 'acceDeleteRole',
      desc: '',
      args: [rName],
    );
  }

  /// `Roles still assigned to users cannot be deleted.`
  String get acceRolesStillAssignedToUsers {
    return Intl.message(
      'Roles still assigned to users cannot be deleted.',
      name: 'acceRolesStillAssignedToUsers',
      desc: '',
      args: [],
    );
  }

  /// `Built-in roles cannot be changed. Clone it to make your own version.`
  String get acceBuiltInRolesCannotBe {
    return Intl.message(
      'Built-in roles cannot be changed. Clone it to make your own version.',
      name: 'acceBuiltInRolesCannotBe',
      desc: '',
      args: [],
    );
  }

  /// `Description`
  String get acceDescription {
    return Intl.message(
      'Description',
      name: 'acceDescription',
      desc: '',
      args: [],
    );
  }

  /// `all / none`
  String get acceAllNone {
    return Intl.message('all / none', name: 'acceAllNone', desc: '', args: []);
  }

  /// `"Global only" permissions have no effect when the role is given for one project.`
  String get acceGlobalOnlyPermissionsHaveNo {
    return Intl.message(
      '"Global only" permissions have no effect when the role is given for one project.',
      name: 'acceGlobalOnlyPermissionsHaveNo',
      desc: '',
      args: [],
    );
  }

  /// `Clone`
  String get acceClone {
    return Intl.message('Clone', name: 'acceClone', desc: '', args: []);
  }

  /// `API tokens`
  String get acceApiTokens {
    return Intl.message(
      'API tokens',
      name: 'acceApiTokens',
      desc: '',
      args: [],
    );
  }

  /// `For CI and scripts. A token acts as you, limited to a role and a project when you choose them.`
  String get acceForCiAndScriptsA {
    return Intl.message(
      'For CI and scripts. A token acts as you, limited to a role and a project when you choose them.',
      name: 'acceForCiAndScriptsA',
      desc: '',
      args: [],
    );
  }

  /// `New token`
  String get acceNewToken {
    return Intl.message('New token', name: 'acceNewToken', desc: '', args: []);
  }

  /// `Your tokens`
  String get acceYourTokens {
    return Intl.message(
      'Your tokens',
      name: 'acceYourTokens',
      desc: '',
      args: [],
    );
  }

  /// `No tokens yet`
  String get acceNoTokensYet {
    return Intl.message(
      'No tokens yet',
      name: 'acceNoTokensYet',
      desc: '',
      args: [],
    );
  }

  /// `Create one for GitHub Actions or the nectar CLI. A Deployer token for one project is enough to deploy.`
  String get acceCreateOneForGithubActions {
    return Intl.message(
      'Create one for GitHub Actions or the nectar CLI. A Deployer token for one project is enough to deploy.',
      name: 'acceCreateOneForGithubActions',
      desc: '',
      args: [],
    );
  }

  /// `Everyone's tokens`
  String get acceEveryoneSTokens {
    return Intl.message(
      'Everyone\'s tokens',
      name: 'acceEveryoneSTokens',
      desc: '',
      args: [],
    );
  }

  /// `No tokens.`
  String get acceNoTokens {
    return Intl.message('No tokens.', name: 'acceNoTokens', desc: '', args: []);
  }

  /// `Revoke {tName}?`
  String acceRevoke(Object tName) {
    return Intl.message(
      'Revoke $tName?',
      name: 'acceRevoke',
      desc: '',
      args: [tName],
    );
  }

  /// `Scripts and pipelines that use it stop working immediately.`
  String get acceScriptsAndPipelinesThatUse {
    return Intl.message(
      'Scripts and pipelines that use it stop working immediately.',
      name: 'acceScriptsAndPipelinesThatUse',
      desc: '',
      args: [],
    );
  }

  /// `Revoke`
  String get acceRevoke2 {
    return Intl.message('Revoke', name: 'acceRevoke2', desc: '', args: []);
  }

  /// `Using a token`
  String get acceUsingAToken {
    return Intl.message(
      'Using a token',
      name: 'acceUsingAToken',
      desc: '',
      args: [],
    );
  }

  /// `Any HTTP client works too: Authorization: Bearer nat_… on the API (see api/openapi.yaml).`
  String get acceAnyHttpClientWorksToo {
    return Intl.message(
      'Any HTTP client works too: Authorization: Bearer nat_… on the API (see api/openapi.yaml).',
      name: 'acceAnyHttpClientWorksToo',
      desc: '',
      args: [],
    );
  }

  /// `Copy your token`
  String get acceCopyYourToken {
    return Intl.message(
      'Copy your token',
      name: 'acceCopyYourToken',
      desc: '',
      args: [],
    );
  }

  /// `It is shown only now. Store it as a secret (e.g. NECTAR_TOKEN in GitHub).`
  String get acceItIsShownOnlyNow {
    return Intl.message(
      'It is shown only now. Store it as a secret (e.g. NECTAR_TOKEN in GitHub).',
      name: 'acceItIsShownOnlyNow',
      desc: '',
      args: [],
    );
  }

  /// `New API token`
  String get acceNewApiToken {
    return Intl.message(
      'New API token',
      name: 'acceNewApiToken',
      desc: '',
      args: [],
    );
  }

  /// `GitHub Actions: shop`
  String get acceGithubActionsShop {
    return Intl.message(
      'GitHub Actions: shop',
      name: 'acceGithubActionsShop',
      desc: '',
      args: [],
    );
  }

  /// `Rights`
  String get acceRights {
    return Intl.message('Rights', name: 'acceRights', desc: '', args: []);
  }

  /// `Everything you may do`
  String get acceEverythingYouMayDo {
    return Intl.message(
      'Everything you may do',
      name: 'acceEverythingYouMayDo',
      desc: '',
      args: [],
    );
  }

  /// `Only {rName}`
  String acceOnly(Object rName) {
    return Intl.message(
      'Only $rName',
      name: 'acceOnly',
      desc: '',
      args: [rName],
    );
  }

  /// `Only {pName}`
  String acceOnly2(Object pName) {
    return Intl.message(
      'Only $pName',
      name: 'acceOnly2',
      desc: '',
      args: [pName],
    );
  }

  /// `Expires`
  String get acceExpires {
    return Intl.message('Expires', name: 'acceExpires', desc: '', args: []);
  }

  /// `in 30 days`
  String get acceIn30Days {
    return Intl.message('in 30 days', name: 'acceIn30Days', desc: '', args: []);
  }

  /// `in 90 days`
  String get acceIn90Days {
    return Intl.message('in 90 days', name: 'acceIn90Days', desc: '', args: []);
  }

  /// `in a year`
  String get acceInAYear {
    return Intl.message('in a year', name: 'acceInAYear', desc: '', args: []);
  }

  /// `A token never has more rights than you. For CI: the Deployer role and one project.`
  String get acceATokenNeverHasMore {
    return Intl.message(
      'A token never has more rights than you. For CI: the Deployer role and one project.',
      name: 'acceATokenNeverHasMore',
      desc: '',
      args: [],
    );
  }

  /// `Users`
  String get acceUsers {
    return Intl.message('Users', name: 'acceUsers', desc: '', args: []);
  }

  /// `Your role does not include user.manage.`
  String get acceYourRoleDoesNotInclude {
    return Intl.message(
      'Your role does not include user.manage.',
      name: 'acceYourRoleDoesNotInclude',
      desc: '',
      args: [],
    );
  }

  /// `Who can sign in, and what they may do. Roles apply everywhere or to one project.`
  String get acceWhoCanSignInAnd {
    return Intl.message(
      'Who can sign in, and what they may do. Roles apply everywhere or to one project.',
      name: 'acceWhoCanSignInAnd',
      desc: '',
      args: [],
    );
  }

  /// `Add user`
  String get acceAddUser {
    return Intl.message('Add user', name: 'acceAddUser', desc: '', args: []);
  }

  /// `User created`
  String get acceUserCreated {
    return Intl.message(
      'User created',
      name: 'acceUserCreated',
      desc: '',
      args: [],
    );
  }

  /// `Share the invitation link`
  String get acceShareTheInvitationLink {
    return Intl.message(
      'Share the invitation link',
      name: 'acceShareTheInvitationLink',
      desc: '',
      args: [],
    );
  }

  /// `The panel cannot send emails (Settings → Email). Send this link yourself; it works for 7 days.`
  String get acceThePanelCannotSendEmails {
    return Intl.message(
      'The panel cannot send emails (Settings → Email). Send this link yourself; it works for 7 days.',
      name: 'acceThePanelCannotSendEmails',
      desc: '',
      args: [],
    );
  }

  /// `Send an invitation`
  String get acceSendAnInvitation {
    return Intl.message(
      'Send an invitation',
      name: 'acceSendAnInvitation',
      desc: '',
      args: [],
    );
  }

  /// `They choose their own password`
  String get acceTheyChooseTheirOwnPassword {
    return Intl.message(
      'They choose their own password',
      name: 'acceTheyChooseTheirOwnPassword',
      desc: '',
      args: [],
    );
  }

  /// `Password (10+ characters)`
  String get accePassword10Characters {
    return Intl.message(
      'Password (10+ characters)',
      name: 'accePassword10Characters',
      desc: '',
      args: [],
    );
  }

  /// `New users have no roles: give them one after creating them.`
  String get acceNewUsersHaveNoRoles {
    return Intl.message(
      'New users have no roles: give them one after creating them.',
      name: 'acceNewUsersHaveNoRoles',
      desc: '',
      args: [],
    );
  }

  /// `Delete {label}?`
  String acceDelete(Object label) {
    return Intl.message(
      'Delete $label?',
      name: 'acceDelete',
      desc: '',
      args: [label],
    );
  }

  /// `They can no longer sign in; their API tokens stop working. The audit log keeps their actions.`
  String get acceTheyCanNoLongerSign {
    return Intl.message(
      'They can no longer sign in; their API tokens stop working. The audit log keeps their actions.',
      name: 'acceTheyCanNoLongerSign',
      desc: '',
      args: [],
    );
  }

  /// `Disabled`
  String get acceDisabled {
    return Intl.message('Disabled', name: 'acceDisabled', desc: '', args: []);
  }

  /// `Cannot sign in; sessions and tokens stop working`
  String get acceCannotSignInSessionsAnd {
    return Intl.message(
      'Cannot sign in; sessions and tokens stop working',
      name: 'acceCannotSignInSessionsAnd',
      desc: '',
      args: [],
    );
  }

  /// `Owners can do everything; roles do not change that.`
  String get acceOwnersCanDoEverythingRoles {
    return Intl.message(
      'Owners can do everything; roles do not change that.',
      name: 'acceOwnersCanDoEverythingRoles',
      desc: '',
      args: [],
    );
  }

  /// `Role`
  String get acceRole {
    return Intl.message('Role', name: 'acceRole', desc: '', args: []);
  }

  /// `Where`
  String get acceWhere {
    return Intl.message('Where', name: 'acceWhere', desc: '', args: []);
  }

  /// `Everywhere`
  String get acceEverywhere {
    return Intl.message(
      'Everywhere',
      name: 'acceEverywhere',
      desc: '',
      args: [],
    );
  }

  /// `Add role`
  String get acceAddRole {
    return Intl.message('Add role', name: 'acceAddRole', desc: '', args: []);
  }

  /// `You can only give or take away roles whose permissions you hold yourself.`
  String get acceYouCanOnlyGiveOr {
    return Intl.message(
      'You can only give or take away roles whose permissions you hold yourself.',
      name: 'acceYouCanOnlyGiveOr',
      desc: '',
      args: [],
    );
  }

  /// `Reset 2FA`
  String get acceReset2fa {
    return Intl.message('Reset 2FA', name: 'acceReset2fa', desc: '', args: []);
  }

  /// `Send invitation / reset link`
  String get acceSendInvitationResetLink {
    return Intl.message(
      'Send invitation / reset link',
      name: 'acceSendInvitationResetLink',
      desc: '',
      args: [],
    );
  }

  /// `Added {createdAt}`
  String acceAdded(Object createdAt) {
    return Intl.message(
      'Added $createdAt',
      name: 'acceAdded',
      desc: '',
      args: [createdAt],
    );
  }

  /// `Every domain of every app you can see, with its HTTPS and DNS state.`
  String get domainEveryDomainOfEveryApp {
    return Intl.message(
      'Every domain of every app you can see, with its HTTPS and DNS state.',
      name: 'domainEveryDomainOfEveryApp',
      desc: '',
      args: [],
    );
  }

  /// `Search domains, apps, projects`
  String get domainSearchDomainsAppsProjects {
    return Intl.message(
      'Search domains, apps, projects',
      name: 'domainSearchDomainsAppsProjects',
      desc: '',
      args: [],
    );
  }

  /// `All`
  String get domainAll {
    return Intl.message('All', name: 'domainAll', desc: '', args: []);
  }

  /// `Custom`
  String get domainCustom {
    return Intl.message('Custom', name: 'domainCustom', desc: '', args: []);
  }

  /// `Problems`
  String get domainProblems {
    return Intl.message('Problems', name: 'domainProblems', desc: '', args: []);
  }

  /// `No domains yet`
  String get domainNoDomainsYet {
    return Intl.message(
      'No domains yet',
      name: 'domainNoDomainsYet',
      desc: '',
      args: [],
    );
  }

  /// `Web apps get a domain under the base domain when they are created; add your own on the app page.`
  String get domainWebAppsGetADomain {
    return Intl.message(
      'Web apps get a domain under the base domain when they are created; add your own on the app page.',
      name: 'domainWebAppsGetADomain',
      desc: '',
      args: [],
    );
  }

  /// `Nothing matches.`
  String get domainNothingMatches {
    return Intl.message(
      'Nothing matches.',
      name: 'domainNothingMatches',
      desc: '',
      args: [],
    );
  }

  /// `Automatic DNS records off`
  String get domainAutomaticDnsRecordsOff {
    return Intl.message(
      'Automatic DNS records off',
      name: 'domainAutomaticDnsRecordsOff',
      desc: '',
      args: [],
    );
  }

  /// `DNS setup`
  String get domainDnsSetup {
    return Intl.message(
      'DNS setup',
      name: 'domainDnsSetup',
      desc: '',
      args: [],
    );
  }

  /// `The server's public IP is unknown. Set it in the setup wizard (Settings → Domain, HTTPS email and sample app) so the records can be shown and checked.`
  String get domainTheServerSPublicIp {
    return Intl.message(
      'The server\'s public IP is unknown. Set it in the setup wizard (Settings → Domain, HTTPS email and sample app) so the records can be shown and checked.',
      name: 'domainTheServerSPublicIp',
      desc: '',
      args: [],
    );
  }

  /// `Point domains at this server with these records at your DNS provider:`
  String get domainPointDomainsAtThisServer {
    return Intl.message(
      'Point domains at this server with these records at your DNS provider:',
      name: 'domainPointDomainsAtThisServer',
      desc: '',
      args: [],
    );
  }

  /// `The *.{baseDomain} record serves every app's default domain. A CNAME to {baseHost} works for subdomains of your own domains once it exists.`
  String domainTheRecordServesEveryApp(Object baseDomain, Object baseHost) {
    return Intl.message(
      'The *.$baseDomain record serves every app\'s default domain. A CNAME to $baseHost works for subdomains of your own domains once it exists.',
      name: 'domainTheRecordServesEveryApp',
      desc: '',
      args: [baseDomain, baseHost],
    );
  }

  /// `Create DNS records automatically`
  String get domainCreateDnsRecordsAutomatically {
    return Intl.message(
      'Create DNS records automatically',
      name: 'domainCreateDnsRecordsAutomatically',
      desc: '',
      args: [],
    );
  }

  /// `Actions`
  String get domainActions {
    return Intl.message('Actions', name: 'domainActions', desc: '', args: []);
  }

  /// `Open app`
  String get domainOpenApp {
    return Intl.message('Open app', name: 'domainOpenApp', desc: '', args: []);
  }

  /// `Check DNS`
  String get domainCheckDns {
    return Intl.message(
      'Check DNS',
      name: 'domainCheckDns',
      desc: '',
      args: [],
    );
  }

  /// `Database`
  String get addonDatabase {
    return Intl.message('Database', name: 'addonDatabase', desc: '', args: []);
  }

  /// `Delete {addonName}?`
  String addonDelete(Object addonName) {
    return Intl.message(
      'Delete $addonName?',
      name: 'addonDelete',
      desc: '',
      args: [addonName],
    );
  }

  /// `The database and all its data are deleted for good. Linked apps lose their connection settings with their next deploy.`
  String get addonTheDatabaseAndAllIts {
    return Intl.message(
      'The database and all its data are deleted for good. Linked apps lose their connection settings with their next deploy.',
      name: 'addonTheDatabaseAndAllIts',
      desc: '',
      args: [],
    );
  }

  /// `Delete database`
  String get addonDeleteDatabase {
    return Intl.message(
      'Delete database',
      name: 'addonDeleteDatabase',
      desc: '',
      args: [],
    );
  }

  /// `Connection`
  String get addonConnection {
    return Intl.message(
      'Connection',
      name: 'addonConnection',
      desc: '',
      args: [],
    );
  }

  /// `Show credentials`
  String get addonShowCredentials {
    return Intl.message(
      'Show credentials',
      name: 'addonShowCredentials',
      desc: '',
      args: [],
    );
  }

  /// `Reachable only from apps in the same project, at {host}:{port}. Link an app below to give it these settings as variables.`
  String addonReachableOnlyFromAppsIn(Object host, Object port) {
    return Intl.message(
      'Reachable only from apps in the same project, at $host:$port. Link an app below to give it these settings as variables.',
      name: 'addonReachableOnlyFromAppsIn',
      desc: '',
      args: [host, port],
    );
  }

  /// `Every app of the project is linked already`
  String get addonEveryAppOfTheProject {
    return Intl.message(
      'Every app of the project is linked already',
      name: 'addonEveryAppOfTheProject',
      desc: '',
      args: [],
    );
  }

  /// `Link {addonName} to an app`
  String addonLinkToAnApp(Object addonName) {
    return Intl.message(
      'Link $addonName to an app',
      name: 'addonLinkToAnApp',
      desc: '',
      args: [addonName],
    );
  }

  /// `App`
  String get addonApp {
    return Intl.message('App', name: 'addonApp', desc: '', args: []);
  }

  /// `Variable prefix (optional)`
  String get addonVariablePrefixOptional {
    return Intl.message(
      'Variable prefix (optional)',
      name: 'addonVariablePrefixOptional',
      desc: '',
      args: [],
    );
  }

  /// `CACHE_`
  String get addonCache {
    return Intl.message('CACHE_', name: 'addonCache', desc: '', args: []);
  }

  /// `For a second database: CACHE_ gives CACHE_DATABASE_URL`
  String get addonForASecondDatabaseCache {
    return Intl.message(
      'For a second database: CACHE_ gives CACHE_DATABASE_URL',
      name: 'addonForASecondDatabaseCache',
      desc: '',
      args: [],
    );
  }

  /// `Link`
  String get addonLink {
    return Intl.message('Link', name: 'addonLink', desc: '', args: []);
  }

  /// `Linked. Redeploy {appName} to apply.`
  String addonLinkedRedeployToApply(Object appName) {
    return Intl.message(
      'Linked. Redeploy $appName to apply.',
      name: 'addonLinkedRedeployToApply',
      desc: '',
      args: [appName],
    );
  }

  /// `Linked apps`
  String get addonLinkedApps {
    return Intl.message(
      'Linked apps',
      name: 'addonLinkedApps',
      desc: '',
      args: [],
    );
  }

  /// `Link an app`
  String get addonLinkAnApp {
    return Intl.message(
      'Link an app',
      name: 'addonLinkAnApp',
      desc: '',
      args: [],
    );
  }

  /// `No app uses this database yet.`
  String get addonNoAppUsesThisDatabase {
    return Intl.message(
      'No app uses this database yet.',
      name: 'addonNoAppUsesThisDatabase',
      desc: '',
      args: [],
    );
  }

  /// `Unlink`
  String get addonUnlink {
    return Intl.message('Unlink', name: 'addonUnlink', desc: '', args: []);
  }

  /// `Saved; the database restarts with the new resources`
  String get addonSavedTheDatabaseRestartsWith {
    return Intl.message(
      'Saved; the database restarts with the new resources',
      name: 'addonSavedTheDatabaseRestartsWith',
      desc: '',
      args: [],
    );
  }

  /// `Add database`
  String get addonAddDatabase {
    return Intl.message(
      'Add database',
      name: 'addonAddDatabase',
      desc: '',
      args: [],
    );
  }

  /// `PostgreSQL, MySQL, MariaDB or Redis with a persistent volume. Link it to an app and it gets DATABASE_URL (or REDIS_URL).`
  String get addonPostgresqlMysqlMariadbOrRedis {
    return Intl.message(
      'PostgreSQL, MySQL, MariaDB or Redis with a persistent volume. Link it to an app and it gets DATABASE_URL (or REDIS_URL).',
      name: 'addonPostgresqlMysqlMariadbOrRedis',
      desc: '',
      args: [],
    );
  }

  /// `Add a database`
  String get addonAddADatabase {
    return Intl.message(
      'Add a database',
      name: 'addonAddADatabase',
      desc: '',
      args: [],
    );
  }

  /// `Name (also the host name)`
  String get addonNameAlsoTheHostName {
    return Intl.message(
      'Name (also the host name)',
      name: 'addonNameAlsoTheHostName',
      desc: '',
      args: [],
    );
  }

  /// `Lowercase letters, digits, dashes`
  String get addonLowercaseLettersDigitsDashes {
    return Intl.message(
      'Lowercase letters, digits, dashes',
      name: 'addonLowercaseLettersDigitsDashes',
      desc: '',
      args: [],
    );
  }

  /// `Storage (MiB)`
  String get addonStorageMib {
    return Intl.message(
      'Storage (MiB)',
      name: 'addonStorageMib',
      desc: '',
      args: [],
    );
  }

  /// `Memory (MiB)`
  String get addonMemoryMib {
    return Intl.message(
      'Memory (MiB)',
      name: 'addonMemoryMib',
      desc: '',
      args: [],
    );
  }

  /// `At least {minMemoryMb}`
  String addonAtLeast(Object minMemoryMb) {
    return Intl.message(
      'At least $minMemoryMb',
      name: 'addonAtLeast',
      desc: '',
      args: [minMemoryMb],
    );
  }

  /// `Push notifications`
  String get settingPushNotifications {
    return Intl.message(
      'Push notifications',
      name: 'settingPushNotifications',
      desc: '',
      args: [],
    );
  }

  /// `Optional. Alerts reach the Nectar app on Android, iPhone and Mac through your own Firebase project (free). Without it, use ntfy, Telegram or email channels under Alerts.`
  String get settingOptionalAlertsReachTheNectar {
    return Intl.message(
      'Optional. Alerts reach the Nectar app on Android, iPhone and Mac through your own Firebase project (free). Without it, use ntfy, Telegram or email channels under Alerts.',
      name: 'settingOptionalAlertsReachTheNectar',
      desc: '',
      args: [],
    );
  }

  /// `Send test to my devices`
  String get settingSendTestToMyDevices {
    return Intl.message(
      'Send test to my devices',
      name: 'settingSendTestToMyDevices',
      desc: '',
      args: [],
    );
  }

  /// `1. Create a Firebase project (console.firebase.google.com) and add an Android app and/or an Apple app with the Nectar app's package / bundle id.\n2. iPhone and Mac: upload your APNs key under Project settings → Cloud Messaging.\n3. Copy the values below from Project settings, then add a "Mobile app" channel under Alerts.`
  String get setting1CreateAFirebaseProject {
    return Intl.message(
      '1. Create a Firebase project (console.firebase.google.com) and add an Android app and/or an Apple app with the Nectar app\'s package / bundle id.\n2. iPhone and Mac: upload your APNs key under Project settings → Cloud Messaging.\n3. Copy the values below from Project settings, then add a "Mobile app" channel under Alerts.',
      name: 'setting1CreateAFirebaseProject',
      desc: '',
      args: [],
    );
  }

  /// `Send push notifications`
  String get settingSendPushNotifications {
    return Intl.message(
      'Send push notifications',
      name: 'settingSendPushNotifications',
      desc: '',
      args: [],
    );
  }

  /// `Service account key (JSON)`
  String get settingServiceAccountKeyJson {
    return Intl.message(
      'Service account key (JSON)',
      name: 'settingServiceAccountKeyJson',
      desc: '',
      args: [],
    );
  }

  /// `Project settings → Service accounts → Generate new private key. Stored encrypted.`
  String get settingProjectSettingsServiceAccountsGenerate {
    return Intl.message(
      'Project settings → Service accounts → Generate new private key. Stored encrypted.',
      name: 'settingProjectSettingsServiceAccountsGenerate',
      desc: '',
      args: [],
    );
  }

  /// `Sender ID`
  String get settingSenderId {
    return Intl.message(
      'Sender ID',
      name: 'settingSenderId',
      desc: '',
      args: [],
    );
  }

  /// `Project settings → Cloud Messaging`
  String get settingProjectSettingsCloudMessaging {
    return Intl.message(
      'Project settings → Cloud Messaging',
      name: 'settingProjectSettingsCloudMessaging',
      desc: '',
      args: [],
    );
  }

  /// `App ID`
  String get settingAppId {
    return Intl.message('App ID', name: 'settingAppId', desc: '', args: []);
  }

  /// `1:123456789:android:…`
  String get setting1123456789Android {
    return Intl.message(
      '1:123456789:android:…',
      name: 'setting1123456789Android',
      desc: '',
      args: [],
    );
  }

  /// `API key`
  String get settingApiKey {
    return Intl.message('API key', name: 'settingApiKey', desc: '', args: []);
  }

  /// `AIza…`
  String get settingAiza {
    return Intl.message('AIza…', name: 'settingAiza', desc: '', args: []);
  }

  /// `1:123456789:ios:…`
  String get setting1123456789Ios {
    return Intl.message(
      '1:123456789:ios:…',
      name: 'setting1123456789Ios',
      desc: '',
      args: [],
    );
  }

  /// `Bundle ID`
  String get settingBundleId {
    return Intl.message(
      'Bundle ID',
      name: 'settingBundleId',
      desc: '',
      args: [],
    );
  }

  /// `as in Xcode (needed for iPhone)`
  String get settingAsInXcodeNeededFor {
    return Intl.message(
      'as in Xcode (needed for iPhone)',
      name: 'settingAsInXcodeNeededFor',
      desc: '',
      args: [],
    );
  }

  /// `Remove the login for {host}?`
  String settingRemoveTheLoginFor(Object host) {
    return Intl.message(
      'Remove the login for $host?',
      name: 'settingRemoveTheLoginFor',
      desc: '',
      args: [host],
    );
  }

  /// `Apps can no longer pull private images from it.`
  String get settingAppsCanNoLongerPull {
    return Intl.message(
      'Apps can no longer pull private images from it.',
      name: 'settingAppsCanNoLongerPull',
      desc: '',
      args: [],
    );
  }

  /// `Registries`
  String get settingRegistries {
    return Intl.message(
      'Registries',
      name: 'settingRegistries',
      desc: '',
      args: [],
    );
  }

  /// `Add login`
  String get settingAddLogin {
    return Intl.message(
      'Add login',
      name: 'settingAddLogin',
      desc: '',
      args: [],
    );
  }

  /// `No private registry logins. Public images (Docker Hub, GHCR public packages, …) need none.`
  String get settingNoPrivateRegistryLoginsPublic {
    return Intl.message(
      'No private registry logins. Public images (Docker Hub, GHCR public packages, …) need none.',
      name: 'settingNoPrivateRegistryLoginsPublic',
      desc: '',
      args: [],
    );
  }

  /// `{username} · changed {updatedAt}`
  String settingChanged(Object username, Object updatedAt) {
    return Intl.message(
      '$username · changed $updatedAt',
      name: 'settingChanged',
      desc: '',
      args: [username, updatedAt],
    );
  }

  /// `Logins are stored encrypted and given to every project as an image pull secret; builds use them for base images.`
  String get settingLoginsAreStoredEncryptedAnd {
    return Intl.message(
      'Logins are stored encrypted and given to every project as an image pull secret; builds use them for base images.',
      name: 'settingLoginsAreStoredEncryptedAnd',
      desc: '',
      args: [],
    );
  }

  /// `Registry login`
  String get settingRegistryLogin {
    return Intl.message(
      'Registry login',
      name: 'settingRegistryLogin',
      desc: '',
      args: [],
    );
  }

  /// `Registry`
  String get settingRegistry {
    return Intl.message(
      'Registry',
      name: 'settingRegistry',
      desc: '',
      args: [],
    );
  }

  /// `ghcr.io, docker.io, registry.gitlab.com`
  String get settingGhcrIoDockerIoRegistry {
    return Intl.message(
      'ghcr.io, docker.io, registry.gitlab.com',
      name: 'settingGhcrIoDockerIoRegistry',
      desc: '',
      args: [],
    );
  }

  /// `User name`
  String get settingUserName {
    return Intl.message(
      'User name',
      name: 'settingUserName',
      desc: '',
      args: [],
    );
  }

  /// `Password or access token`
  String get settingPasswordOrAccessToken {
    return Intl.message(
      'Password or access token',
      name: 'settingPasswordOrAccessToken',
      desc: '',
      args: [],
    );
  }

  /// `GHCR: a token with read:packages`
  String get settingGhcrATokenWithRead {
    return Intl.message(
      'GHCR: a token with read:packages',
      name: 'settingGhcrATokenWithRead',
      desc: '',
      args: [],
    );
  }

  /// `Your password`
  String get settingYourPassword {
    return Intl.message(
      'Your password',
      name: 'settingYourPassword',
      desc: '',
      args: [],
    );
  }

  /// `Two-factor authentication is off`
  String get settingTwoFactorAuthenticationIsOff {
    return Intl.message(
      'Two-factor authentication is off',
      name: 'settingTwoFactorAuthenticationIsOff',
      desc: '',
      args: [],
    );
  }

  /// `Two-factor authentication`
  String get settingTwoFactorAuthentication {
    return Intl.message(
      'Two-factor authentication',
      name: 'settingTwoFactorAuthentication',
      desc: '',
      args: [],
    );
  }

  /// `Only {left} recovery codes left: create new ones.`
  String settingOnlyRecoveryCodesLeftCreate(Object left) {
    return Intl.message(
      'Only $left recovery codes left: create new ones.',
      name: 'settingOnlyRecoveryCodesLeftCreate',
      desc: '',
      args: [left],
    );
  }

  /// `New recovery codes`
  String get settingNewRecoveryCodes {
    return Intl.message(
      'New recovery codes',
      name: 'settingNewRecoveryCodes',
      desc: '',
      args: [],
    );
  }

  /// `Turn off`
  String get settingTurnOff {
    return Intl.message('Turn off', name: 'settingTurnOff', desc: '', args: []);
  }

  /// `Set up`
  String get settingSetUp {
    return Intl.message('Set up', name: 'settingSetUp', desc: '', args: []);
  }

  /// `Recovery codes`
  String get settingRecoveryCodes {
    return Intl.message(
      'Recovery codes',
      name: 'settingRecoveryCodes',
      desc: '',
      args: [],
    );
  }

  /// `Each code signs you in once if your phone is not at hand. Store them in your password manager: they are shown only now.`
  String get settingEachCodeSignsYouIn {
    return Intl.message(
      'Each code signs you in once if your phone is not at hand. Store them in your password manager: they are shown only now.',
      name: 'settingEachCodeSignsYouIn',
      desc: '',
      args: [],
    );
  }

  /// `I saved them`
  String get settingISavedThem {
    return Intl.message(
      'I saved them',
      name: 'settingISavedThem',
      desc: '',
      args: [],
    );
  }

  /// `Set up two-factor authentication`
  String get settingSetUpTwoFactorAuthentication {
    return Intl.message(
      'Set up two-factor authentication',
      name: 'settingSetUpTwoFactorAuthentication',
      desc: '',
      args: [],
    );
  }

  /// `1. Scan this code with your authenticator app.`
  String get setting1ScanThisCodeWith {
    return Intl.message(
      '1. Scan this code with your authenticator app.',
      name: 'setting1ScanThisCodeWith',
      desc: '',
      args: [],
    );
  }

  /// `Or enter the key by hand:`
  String get settingOrEnterTheKeyBy {
    return Intl.message(
      'Or enter the key by hand:',
      name: 'settingOrEnterTheKeyBy',
      desc: '',
      args: [],
    );
  }

  /// `2. Enter the 6-digit code it shows.`
  String get setting2EnterThe6Digit {
    return Intl.message(
      '2. Enter the 6-digit code it shows.',
      name: 'setting2EnterThe6Digit',
      desc: '',
      args: [],
    );
  }

  /// `Signed in`
  String get settingSignedIn {
    return Intl.message(
      'Signed in',
      name: 'settingSignedIn',
      desc: '',
      args: [],
    );
  }

  /// `{ip} · active {lastSeenAt} · since {createdAt}`
  String settingActiveSince(Object ip, Object lastSeenAt, Object createdAt) {
    return Intl.message(
      '$ip · active $lastSeenAt · since $createdAt',
      name: 'settingActiveSince',
      desc: '',
      args: [ip, lastSeenAt, createdAt],
    );
  }

  /// `Test email sent to your address`
  String get settingTestEmailSentToYour {
    return Intl.message(
      'Test email sent to your address',
      name: 'settingTestEmailSentToYour',
      desc: '',
      args: [],
    );
  }

  /// `Email settings saved`
  String get settingEmailSettingsSaved {
    return Intl.message(
      'Email settings saved',
      name: 'settingEmailSettingsSaved',
      desc: '',
      args: [],
    );
  }

  /// `Used for password resets, invitations and alert emails.`
  String get settingUsedForPasswordResetsInvitations {
    return Intl.message(
      'Used for password resets, invitations and alert emails.',
      name: 'settingUsedForPasswordResetsInvitations',
      desc: '',
      args: [],
    );
  }

  /// `STARTTLS`
  String get settingStarttls {
    return Intl.message(
      'STARTTLS',
      name: 'settingStarttls',
      desc: '',
      args: [],
    );
  }

  /// `TLS`
  String get settingTls {
    return Intl.message('TLS', name: 'settingTls', desc: '', args: []);
  }

  /// `No encryption`
  String get settingNoEncryption {
    return Intl.message(
      'No encryption',
      name: 'settingNoEncryption',
      desc: '',
      args: [],
    );
  }

  /// `Nectar Apps <panel@example.com>`
  String get settingNectarAppsPanelExampleCom {
    return Intl.message(
      'Nectar Apps <panel@example.com>',
      name: 'settingNectarAppsPanelExampleCom',
      desc: '',
      args: [],
    );
  }

  /// `Save and send a test`
  String get settingSaveAndSendATest {
    return Intl.message(
      'Save and send a test',
      name: 'settingSaveAndSendATest',
      desc: '',
      args: [],
    );
  }

  /// `Panel access`
  String get settingPanelAccess {
    return Intl.message(
      'Panel access',
      name: 'settingPanelAccess',
      desc: '',
      args: [],
    );
  }

  /// `Only the listed addresses may use the panel`
  String get settingOnlyTheListedAddressesMay {
    return Intl.message(
      'Only the listed addresses may use the panel',
      name: 'settingOnlyTheListedAddressesMay',
      desc: '',
      args: [],
    );
  }

  /// `Allow only these IP addresses or networks (one per line, e.g. 203.0.113.7 or 203.0.113.0/24). Empty = everyone. Your address: {ip}. An SSH tunnel to the server always works, so a wrong list can be fixed.`
  String settingAllowOnlyTheseIpAddresses(Object ip) {
    return Intl.message(
      'Allow only these IP addresses or networks (one per line, e.g. 203.0.113.7 or 203.0.113.0/24). Empty = everyone. Your address: $ip. An SSH tunnel to the server always works, so a wrong list can be fixed.',
      name: 'settingAllowOnlyTheseIpAddresses',
      desc: '',
      args: [ip],
    );
  }

  /// `Account`
  String get settingAccount {
    return Intl.message('Account', name: 'settingAccount', desc: '', args: []);
  }

  /// `Owner`
  String get settingOwner {
    return Intl.message('Owner', name: 'settingOwner', desc: '', args: []);
  }

  /// `Invite people and give them roles, everywhere or per project`
  String get settingInvitePeopleAndGiveThem {
    return Intl.message(
      'Invite people and give them roles, everywhere or per project',
      name: 'settingInvitePeopleAndGiveThem',
      desc: '',
      args: [],
    );
  }

  /// `Permission sets; clone a built-in role to make your own`
  String get settingPermissionSetsCloneABuilt {
    return Intl.message(
      'Permission sets; clone a built-in role to make your own',
      name: 'settingPermissionSetsCloneABuilt',
      desc: '',
      args: [],
    );
  }

  /// `For the nectar CLI, GitHub Actions and scripts`
  String get settingForTheNectarCliGithub {
    return Intl.message(
      'For the nectar CLI, GitHub Actions and scripts',
      name: 'settingForTheNectarCliGithub',
      desc: '',
      args: [],
    );
  }

  /// `Plans, targets, restore and the recovery kit`
  String get settingPlansTargetsRestoreAndThe {
    return Intl.message(
      'Plans, targets, restore and the recovery kit',
      name: 'settingPlansTargetsRestoreAndThe',
      desc: '',
      args: [],
    );
  }

  /// `Panel, gateway and k3s versions, automatic updates, history and rollback`
  String get settingPanelGatewayAndK3sVersions {
    return Intl.message(
      'Panel, gateway and k3s versions, automatic updates, history and rollback',
      name: 'settingPanelGatewayAndK3sVersions',
      desc: '',
      args: [],
    );
  }

  /// `Rules and notification channels`
  String get settingRulesAndNotificationChannels {
    return Intl.message(
      'Rules and notification channels',
      name: 'settingRulesAndNotificationChannels',
      desc: '',
      args: [],
    );
  }

  /// `Domain, HTTPS email and sample app (setup wizard)`
  String get settingDomainHttpsEmailAndSample {
    return Intl.message(
      'Domain, HTTPS email and sample app (setup wizard)',
      name: 'settingDomainHttpsEmailAndSample',
      desc: '',
      args: [],
    );
  }

  /// `About`
  String get settingAbout {
    return Intl.message('About', name: 'settingAbout', desc: '', args: []);
  }

  /// `Nectar Apps · © TechBrain LLC · Nectar Source License`
  String get settingNectarAppsTechbrainLlcNectar {
    return Intl.message(
      'Nectar Apps · © TechBrain LLC · Nectar Source License',
      name: 'settingNectarAppsTechbrainLlcNectar',
      desc: '',
      args: [],
    );
  }

  /// `Licenses`
  String get settingLicenses {
    return Intl.message(
      'Licenses',
      name: 'settingLicenses',
      desc: '',
      args: [],
    );
  }

  /// `Sign-in providers`
  String get settingSignInProviders {
    return Intl.message(
      'Sign-in providers',
      name: 'settingSignInProviders',
      desc: '',
      args: [],
    );
  }

  /// `Add provider`
  String get settingAddProvider {
    return Intl.message(
      'Add provider',
      name: 'settingAddProvider',
      desc: '',
      args: [],
    );
  }

  /// `Let people sign in with GitHub, Google or your company's identity provider (Authentik, Keycloak, Zitadel, Okta, Entra ID … over OpenID Connect). Works in the web panel.`
  String get settingLetPeopleSignInWith {
    return Intl.message(
      'Let people sign in with GitHub, Google or your company\'s identity provider (Authentik, Keycloak, Zitadel, Okta, Entra ID … over OpenID Connect). Works in the web panel.',
      name: 'settingLetPeopleSignInWith',
      desc: '',
      args: [],
    );
  }

  /// `Remove {pName}?`
  String settingRemove(Object pName) {
    return Intl.message(
      'Remove $pName?',
      name: 'settingRemove',
      desc: '',
      args: [pName],
    );
  }

  /// `Its button disappears from the login page. Users keep their accounts.`
  String get settingItsButtonDisappearsFromThe {
    return Intl.message(
      'Its button disappears from the login page. Users keep their accounts.',
      name: 'settingItsButtonDisappearsFromThe',
      desc: '',
      args: [],
    );
  }

  /// `ID`
  String get settingId {
    return Intl.message('ID', name: 'settingId', desc: '', args: []);
  }

  /// `Button label`
  String get settingButtonLabel {
    return Intl.message(
      'Button label',
      name: 'settingButtonLabel',
      desc: '',
      args: [],
    );
  }

  /// `Company SSO`
  String get settingCompanySso {
    return Intl.message(
      'Company SSO',
      name: 'settingCompanySso',
      desc: '',
      args: [],
    );
  }

  /// `Callback (redirect) URL to register at the provider:`
  String get settingCallbackRedirectUrlToRegister {
    return Intl.message(
      'Callback (redirect) URL to register at the provider:',
      name: 'settingCallbackRedirectUrlToRegister',
      desc: '',
      args: [],
    );
  }

  /// `Issuer URL`
  String get settingIssuerUrl {
    return Intl.message(
      'Issuer URL',
      name: 'settingIssuerUrl',
      desc: '',
      args: [],
    );
  }

  /// `Client ID`
  String get settingClientId {
    return Intl.message(
      'Client ID',
      name: 'settingClientId',
      desc: '',
      args: [],
    );
  }

  /// `Client secret`
  String get settingClientSecret {
    return Intl.message(
      'Client secret',
      name: 'settingClientSecret',
      desc: '',
      args: [],
    );
  }

  /// `Allowed email domains`
  String get settingAllowedEmailDomains {
    return Intl.message(
      'Allowed email domains',
      name: 'settingAllowedEmailDomains',
      desc: '',
      args: [],
    );
  }

  /// `example.com, example.org`
  String get settingExampleComExampleOrg {
    return Intl.message(
      'example.com, example.org',
      name: 'settingExampleComExampleOrg',
      desc: '',
      args: [],
    );
  }

  /// `Empty: only users that already exist can sign in this way (matched by email).`
  String get settingEmptyOnlyUsersThatAlready {
    return Intl.message(
      'Empty: only users that already exist can sign in this way (matched by email).',
      name: 'settingEmptyOnlyUsersThatAlready',
      desc: '',
      args: [],
    );
  }

  /// `Create users on their first sign-in`
  String get settingCreateUsersOnTheirFirst {
    return Intl.message(
      'Create users on their first sign-in',
      name: 'settingCreateUsersOnTheirFirst',
      desc: '',
      args: [],
    );
  }

  /// `Needs allowed domains.`
  String get settingNeedsAllowedDomains {
    return Intl.message(
      'Needs allowed domains.',
      name: 'settingNeedsAllowedDomains',
      desc: '',
      args: [],
    );
  }

  /// `Role of new users (everywhere)`
  String get settingRoleOfNewUsersEverywhere {
    return Intl.message(
      'Role of new users (everywhere)',
      name: 'settingRoleOfNewUsersEverywhere',
      desc: '',
      args: [],
    );
  }

  /// `None (an admin assigns roles)`
  String get settingNoneAnAdminAssignsRoles {
    return Intl.message(
      'None (an admin assigns roles)',
      name: 'settingNoneAnAdminAssignsRoles',
      desc: '',
      args: [],
    );
  }

  /// `Group → role (one per line)`
  String get settingGroupRoleOnePerLine {
    return Intl.message(
      'Group → role (one per line)',
      name: 'settingGroupRoleOnePerLine',
      desc: '',
      args: [],
    );
  }

  /// `nectar-admins=admin\ndevelopers=developer`
  String get settingNectarAdminsAdminDevelopersDeveloper {
    return Intl.message(
      'nectar-admins=admin\ndevelopers=developer',
      name: 'settingNectarAdminsAdminDevelopersDeveloper',
      desc: '',
      args: [],
    );
  }

  /// `Applied at every sign-in from the provider's "groups" claim.`
  String get settingAppliedAtEverySignIn {
    return Intl.message(
      'Applied at every sign-in from the provider\'s "groups" claim.',
      name: 'settingAppliedAtEverySignIn',
      desc: '',
      args: [],
    );
  }

  /// `Show on the login page`
  String get settingShowOnTheLoginPage {
    return Intl.message(
      'Show on the login page',
      name: 'settingShowOnTheLoginPage',
      desc: '',
      args: [],
    );
  }

  /// `Delete {appName}?`
  String appDelete(Object appName) {
    return Intl.message(
      'Delete $appName?',
      name: 'appDelete',
      desc: '',
      args: [appName],
    );
  }

  /// `This removes the app, its domains, releases and uploaded files from the server. It cannot be undone.`
  String get appThisRemovesTheAppIts {
    return Intl.message(
      'This removes the app, its domains, releases and uploaded files from the server. It cannot be undone.',
      name: 'appThisRemovesTheAppIts',
      desc: '',
      args: [],
    );
  }

  /// `Type {slug} to confirm:`
  String appTypeToConfirm(Object slug) {
    return Intl.message(
      'Type $slug to confirm:',
      name: 'appTypeToConfirm',
      desc: '',
      args: [slug],
    );
  }

  /// `Deploy a new version`
  String get appDeployANewVersion {
    return Intl.message(
      'Deploy a new version',
      name: 'appDeployANewVersion',
      desc: '',
      args: [],
    );
  }

  /// `Uploading… {round}%`
  String appUploading(Object round) {
    return Intl.message(
      'Uploading… $round%',
      name: 'appUploading',
      desc: '',
      args: [round],
    );
  }

  /// `{size} uploaded and checked`
  String appUploadedAndChecked(Object size) {
    return Intl.message(
      '$size uploaded and checked',
      name: 'appUploadedAndChecked',
      desc: '',
      args: [size],
    );
  }

  /// `Deploy`
  String get appDeploy {
    return Intl.message('Deploy', name: 'appDeploy', desc: '', args: []);
  }

  /// `Deploy another image`
  String get appDeployAnotherImage {
    return Intl.message(
      'Deploy another image',
      name: 'appDeployAnotherImage',
      desc: '',
      args: [],
    );
  }

  /// `Image`
  String get appImage {
    return Intl.message('Image', name: 'appImage', desc: '', args: []);
  }

  /// `Deploy image`
  String get appDeployImage {
    return Intl.message(
      'Deploy image',
      name: 'appDeployImage',
      desc: '',
      args: [],
    );
  }

  /// `Deploy in progress`
  String get appDeployInProgress {
    return Intl.message(
      'Deploy in progress',
      name: 'appDeployInProgress',
      desc: '',
      args: [],
    );
  }

  /// `Hide`
  String get appHide {
    return Intl.message('Hide', name: 'appHide', desc: '', args: []);
  }

  /// `Not deployed yet`
  String get appNotDeployedYet {
    return Intl.message(
      'Not deployed yet',
      name: 'appNotDeployedYet',
      desc: '',
      args: [],
    );
  }

  /// `More`
  String get appMore {
    return Intl.message('More', name: 'appMore', desc: '', args: []);
  }

  /// `Redeploy current version`
  String get appRedeployCurrentVersion {
    return Intl.message(
      'Redeploy current version',
      name: 'appRedeployCurrentVersion',
      desc: '',
      args: [],
    );
  }

  /// `Restart`
  String get appRestart {
    return Intl.message('Restart', name: 'appRestart', desc: '', args: []);
  }

  /// `Promote to {t}…`
  String appPromoteTo(Object t) {
    return Intl.message(
      'Promote to $t…',
      name: 'appPromoteTo',
      desc: '',
      args: [t],
    );
  }

  /// `Copy app…`
  String get appCopyApp {
    return Intl.message('Copy app…', name: 'appCopyApp', desc: '', args: []);
  }

  /// `Delete app…`
  String get appDeleteApp {
    return Intl.message(
      'Delete app…',
      name: 'appDeleteApp',
      desc: '',
      args: [],
    );
  }

  /// `{ready} / {replicas} ready`
  String appReady(Object ready, Object replicas) {
    return Intl.message(
      '$ready / $replicas ready',
      name: 'appReady',
      desc: '',
      args: [ready, replicas],
    );
  }

  /// `Crash log`
  String get appCrashLog {
    return Intl.message('Crash log', name: 'appCrashLog', desc: '', args: []);
  }

  /// `Port {port}/{proto} is open on the server as {nodePort}`
  String appPortIsOpenOnThe(Object port, Object proto, Object nodePort) {
    return Intl.message(
      'Port $port/$proto is open on the server as $nodePort',
      name: 'appPortIsOpenOnThe',
      desc: '',
      args: [port, proto, nodePort],
    );
  }

  /// `Add domain`
  String get appAddDomain {
    return Intl.message('Add domain', name: 'appAddDomain', desc: '', args: []);
  }

  /// `No domain. Set a base domain in Settings to get one automatically; custom domains come with the domains page.`
  String get appNoDomainSetABase {
    return Intl.message(
      'No domain. Set a base domain in Settings to get one automatically; custom domains come with the domains page.',
      name: 'appNoDomainSetABase',
      desc: '',
      args: [],
    );
  }

  /// `Copied {host}`
  String appCopied(Object host) {
    return Intl.message(
      'Copied $host',
      name: 'appCopied',
      desc: '',
      args: [host],
    );
  }

  /// `Configuration`
  String get appConfiguration {
    return Intl.message(
      'Configuration',
      name: 'appConfiguration',
      desc: '',
      args: [],
    );
  }

  /// `Change these on the Settings tab, variables and files on Environment.`
  String get appChangeTheseOnTheSettings {
    return Intl.message(
      'Change these on the Settings tab, variables and files on Environment.',
      name: 'appChangeTheseOnTheSettings',
      desc: '',
      args: [],
    );
  }

  /// `No deployments yet.`
  String get appNoDeploymentsYet {
    return Intl.message(
      'No deployments yet.',
      name: 'appNoDeploymentsYet',
      desc: '',
      args: [],
    );
  }

  /// `Kubernetes events`
  String get appKubernetesEvents {
    return Intl.message(
      'Kubernetes events',
      name: 'appKubernetesEvents',
      desc: '',
      args: [],
    );
  }

  /// `Source`
  String get appSource {
    return Intl.message('Source', name: 'appSource', desc: '', args: []);
  }

  /// `Build & deploy`
  String get appBuildDeploy {
    return Intl.message(
      'Build & deploy',
      name: 'appBuildDeploy',
      desc: '',
      args: [],
    );
  }

  /// `Builds`
  String get appBuilds {
    return Intl.message('Builds', name: 'appBuilds', desc: '', args: []);
  }

  /// `No build yet.`
  String get appNoBuildYet {
    return Intl.message(
      'No build yet.',
      name: 'appNoBuildYet',
      desc: '',
      args: [],
    );
  }

  /// `Push webhook`
  String get appPushWebhook {
    return Intl.message(
      'Push webhook',
      name: 'appPushWebhook',
      desc: '',
      args: [],
    );
  }

  /// `GitHub: Settings → Webhooks → Add webhook, content type application/json, this secret. GitLab: Webhooks, secret token = this secret, push events. Gitea/Forgejo: the same as GitHub. For preview environments also send pull request (GitLab: merge request) events.`
  String get appGithubSettingsWebhooksAddWebhook {
    return Intl.message(
      'GitHub: Settings → Webhooks → Add webhook, content type application/json, this secret. GitLab: Webhooks, secret token = this secret, push events. Gitea/Forgejo: the same as GitHub. For preview environments also send pull request (GitLab: merge request) events.',
      name: 'appGithubSettingsWebhooksAddWebhook',
      desc: '',
      args: [],
    );
  }

  /// `New webhook secret?`
  String get appNewWebhookSecret {
    return Intl.message(
      'New webhook secret?',
      name: 'appNewWebhookSecret',
      desc: '',
      args: [],
    );
  }

  /// `The old secret stops working: update it in your Git host.`
  String get appTheOldSecretStopsWorking {
    return Intl.message(
      'The old secret stops working: update it in your Git host.',
      name: 'appTheOldSecretStopsWorking',
      desc: '',
      args: [],
    );
  }

  /// `Rotate`
  String get appRotate {
    return Intl.message('Rotate', name: 'appRotate', desc: '', args: []);
  }

  /// `New secret`
  String get appNewSecret {
    return Intl.message('New secret', name: 'appNewSecret', desc: '', args: []);
  }

  /// `Cancel build`
  String get appCancelBuild {
    return Intl.message(
      'Cancel build',
      name: 'appCancelBuild',
      desc: '',
      args: [],
    );
  }

  /// `Remove the saved token (public repository)`
  String get appRemoveTheSavedTokenPublic {
    return Intl.message(
      'Remove the saved token (public repository)',
      name: 'appRemoveTheSavedTokenPublic',
      desc: '',
      args: [],
    );
  }

  /// `Build and deploy on every push`
  String get appBuildAndDeployOnEvery {
    return Intl.message(
      'Build and deploy on every push',
      name: 'appBuildAndDeployOnEvery',
      desc: '',
      args: [],
    );
  }

  /// `Needs the push webhook in your Git host`
  String get appNeedsThePushWebhookIn {
    return Intl.message(
      'Needs the push webhook in your Git host',
      name: 'appNeedsThePushWebhookIn',
      desc: '',
      args: [],
    );
  }

  /// `Repository`
  String get appRepository {
    return Intl.message(
      'Repository',
      name: 'appRepository',
      desc: '',
      args: [],
    );
  }

  /// `Branch`
  String get appBranch {
    return Intl.message('Branch', name: 'appBranch', desc: '', args: []);
  }

  /// `(root)`
  String get appRoot {
    return Intl.message('(root)', name: 'appRoot', desc: '', args: []);
  }

  /// `Dockerfile`
  String get appDockerfile {
    return Intl.message(
      'Dockerfile',
      name: 'appDockerfile',
      desc: '',
      args: [],
    );
  }

  /// `GitHub: a fine-grained token with Contents: read. GitLab: read_repository.`
  String get appGithubAFineGrainedToken {
    return Intl.message(
      'GitHub: a fine-grained token with Contents: read. GitLab: read_repository.',
      name: 'appGithubAFineGrainedToken',
      desc: '',
      args: [],
    );
  }

  /// `The repository needs a Dockerfile. Images are built in the cluster with Kaniko and kept in the built-in registry.`
  String get appTheRepositoryNeedsADockerfile {
    return Intl.message(
      'The repository needs a Dockerfile. Images are built in the cluster with Kaniko and kept in the built-in registry.',
      name: 'appTheRepositoryNeedsADockerfile',
      desc: '',
      args: [],
    );
  }

  /// `Remove {host}?`
  String appRemove(Object host) {
    return Intl.message(
      'Remove $host?',
      name: 'appRemove',
      desc: '',
      args: [host],
    );
  }

  /// `The gateway stops serving it.`
  String get appTheGatewayStopsServingIt {
    return Intl.message(
      'The gateway stops serving it.',
      name: 'appTheGatewayStopsServingIt',
      desc: '',
      args: [],
    );
  }

  /// `Point its DNS (A record) to this server first.`
  String get appPointItsDnsARecord {
    return Intl.message(
      'Point its DNS (A record) to this server first.',
      name: 'appPointItsDnsARecord',
      desc: '',
      args: [],
    );
  }

  /// `Also www. (redirects here)`
  String get appAlsoWwwRedirectsHere {
    return Intl.message(
      'Also www. (redirects here)',
      name: 'appAlsoWwwRedirectsHere',
      desc: '',
      args: [],
    );
  }

  /// `HTTPS (Let's Encrypt)`
  String get appHttpsLetSEncrypt {
    return Intl.message(
      'HTTPS (Let\'s Encrypt)',
      name: 'appHttpsLetSEncrypt',
      desc: '',
      args: [],
    );
  }

  /// `Redirect HTTP to HTTPS`
  String get appRedirectHttpToHttps {
    return Intl.message(
      'Redirect HTTP to HTTPS',
      name: 'appRedirectHttpToHttps',
      desc: '',
      args: [],
    );
  }

  /// `Requests/s per IP`
  String get appRequestsSPerIp {
    return Intl.message(
      'Requests/s per IP',
      name: 'appRequestsSPerIp',
      desc: '',
      args: [],
    );
  }

  /// `Max upload MB`
  String get appMaxUploadMb {
    return Intl.message(
      'Max upload MB',
      name: 'appMaxUploadMb',
      desc: '',
      args: [],
    );
  }

  /// `Timeout s`
  String get appTimeoutS {
    return Intl.message('Timeout s', name: 'appTimeoutS', desc: '', args: []);
  }

  /// `0 = no limit (timeout: 60 s). Above the rate limit a client gets 429 (Too Many Requests); one that keeps going is blocked for a while (Gateway → Blocked IPs). One page load can make dozens of requests. Older gateways block for 24 h at once and need at least 10.`
  String get app0NoLimitTimeout60 {
    return Intl.message(
      '0 = no limit (timeout: 60 s). Above the rate limit a client gets 429 (Too Many Requests); one that keeps going is blocked for a while (Gateway → Blocked IPs). One page load can make dozens of requests. Older gateways block for 24 h at once and need at least 10.',
      name: 'app0NoLimitTimeout60',
      desc: '',
      args: [],
    );
  }

  /// `Redirects, access, headers`
  String get appRedirectsAccessHeaders {
    return Intl.message(
      'Redirects, access, headers',
      name: 'appRedirectsAccessHeaders',
      desc: '',
      args: [],
    );
  }

  /// `Paste a .env file`
  String get appPasteAEnvFile {
    return Intl.message(
      'Paste a .env file',
      name: 'appPasteAEnvFile',
      desc: '',
      args: [],
    );
  }

  /// `DATABASE_URL=postgres://…\nLOG_LEVEL=info`
  String get appDatabaseUrlPostgresLogLevel {
    return Intl.message(
      'DATABASE_URL=postgres://…\nLOG_LEVEL=info',
      name: 'appDatabaseUrlPostgresLogLevel',
      desc: '',
      args: [],
    );
  }

  /// `Add variables`
  String get appAddVariables {
    return Intl.message(
      'Add variables',
      name: 'appAddVariables',
      desc: '',
      args: [],
    );
  }

  /// `No variables. Every app also gets PORT and APP_ARGS.`
  String get appNoVariablesEveryAppAlso {
    return Intl.message(
      'No variables. Every app also gets PORT and APP_ARGS.',
      name: 'appNoVariablesEveryAppAlso',
      desc: '',
      args: [],
    );
  }

  /// `Add variable`
  String get appAddVariable {
    return Intl.message(
      'Add variable',
      name: 'appAddVariable',
      desc: '',
      args: [],
    );
  }

  /// `Paste .env`
  String get appPasteEnv {
    return Intl.message('Paste .env', name: 'appPasteEnv', desc: '', args: []);
  }

  /// `KEY`
  String get appKey {
    return Intl.message('KEY', name: 'appKey', desc: '', args: []);
  }

  /// `Environment variables`
  String get appEnvironmentVariables {
    return Intl.message(
      'Environment variables',
      name: 'appEnvironmentVariables',
      desc: '',
      args: [],
    );
  }

  /// `Config files`
  String get appConfigFiles {
    return Intl.message(
      'Config files',
      name: 'appConfigFiles',
      desc: '',
      args: [],
    );
  }

  /// `No variables.`
  String get appNoVariables {
    return Intl.message(
      'No variables.',
      name: 'appNoVariables',
      desc: '',
      args: [],
    );
  }

  /// `Reveal secrets`
  String get appRevealSecrets {
    return Intl.message(
      'Reveal secrets',
      name: 'appRevealSecrets',
      desc: '',
      args: [],
    );
  }

  /// `Discard`
  String get appDiscard {
    return Intl.message('Discard', name: 'appDiscard', desc: '', args: []);
  }

  /// `Remove {path}?`
  String appRemove2(Object path) {
    return Intl.message(
      'Remove $path?',
      name: 'appRemove2',
      desc: '',
      args: [path],
    );
  }

  /// `The file disappears from the containers with the next deploy.`
  String get appTheFileDisappearsFromThe {
    return Intl.message(
      'The file disappears from the containers with the next deploy.',
      name: 'appTheFileDisappearsFromThe',
      desc: '',
      args: [],
    );
  }

  /// `Mount files such as application.yml, a .env file or a certificate at any path in the container.`
  String get appMountFilesSuchAsApplication {
    return Intl.message(
      'Mount files such as application.yml, a .env file or a certificate at any path in the container.',
      name: 'appMountFilesSuchAsApplication',
      desc: '',
      args: [],
    );
  }

  /// `Add file`
  String get appAddFile {
    return Intl.message('Add file', name: 'appAddFile', desc: '', args: []);
  }

  /// `Path in the container`
  String get appPathInTheContainer {
    return Intl.message(
      'Path in the container',
      name: 'appPathInTheContainer',
      desc: '',
      args: [],
    );
  }

  /// `Content`
  String get appContent {
    return Intl.message('Content', name: 'appContent', desc: '', args: []);
  }

  /// `Secret`
  String get appSecret {
    return Intl.message('Secret', name: 'appSecret', desc: '', args: []);
  }

  /// `Hidden in the panel unless revealed (always encrypted at rest)`
  String get appHiddenInThePanelUnless {
    return Intl.message(
      'Hidden in the panel unless revealed (always encrypted at rest)',
      name: 'appHiddenInThePanelUnless',
      desc: '',
      args: [],
    );
  }

  /// `Save file`
  String get appSaveFile {
    return Intl.message('Save file', name: 'appSaveFile', desc: '', args: []);
  }

  /// `Databases`
  String get appDatabases {
    return Intl.message('Databases', name: 'appDatabases', desc: '', args: []);
  }

  /// `Project databases`
  String get appProjectDatabases {
    return Intl.message(
      'Project databases',
      name: 'appProjectDatabases',
      desc: '',
      args: [],
    );
  }

  /// `No database linked. Add one on the project page and link it to this app.`
  String get appNoDatabaseLinkedAddOne {
    return Intl.message(
      'No database linked. Add one on the project page and link it to this app.',
      name: 'appNoDatabaseLinkedAddOne',
      desc: '',
      args: [],
    );
  }

  /// `Your own variables with the same names win.`
  String get appYourOwnVariablesWithThe {
    return Intl.message(
      'Your own variables with the same names win.',
      name: 'appYourOwnVariablesWithThe',
      desc: '',
      args: [],
    );
  }

  /// `Build started: it deploys {appName} when it is done`
  String appBuildStartedItDeploysWhen(Object appName) {
    return Intl.message(
      'Build started: it deploys $appName when it is done',
      name: 'appBuildStartedItDeploysWhen',
      desc: '',
      args: [appName],
    );
  }

  /// `Leave the wizard?`
  String get appLeaveTheWizard {
    return Intl.message(
      'Leave the wizard?',
      name: 'appLeaveTheWizard',
      desc: '',
      args: [],
    );
  }

  /// `"{appName}" was created but not deployed yet. Keep it to finish later, or discard it.`
  String appWasCreatedButNotDeployed(Object appName) {
    return Intl.message(
      '"$appName" was created but not deployed yet. Keep it to finish later, or discard it.',
      name: 'appWasCreatedButNotDeployed',
      desc: '',
      args: [appName],
    );
  }

  /// `Could not discard: {e}`
  String appCouldNotDiscard(Object e) {
    return Intl.message(
      'Could not discard: $e',
      name: 'appCouldNotDiscard',
      desc: '',
      args: [e],
    );
  }

  /// `Deploy anyway`
  String get appDeployAnyway {
    return Intl.message(
      'Deploy anyway',
      name: 'appDeployAnyway',
      desc: '',
      args: [],
    );
  }

  /// `Step {index} of {stepsLength}`
  String appStepOf(Object index, Object stepsLength) {
    return Intl.message(
      'Step $index of $stepsLength',
      name: 'appStepOf',
      desc: '',
      args: [index, stepsLength],
    );
  }

  /// `{appName} is saved as "not deployed"; you can close the wizard and continue later from the app page.`
  String appIsSavedAsNotDeployed(Object appName) {
    return Intl.message(
      '$appName is saved as "not deployed"; you can close the wizard and continue later from the app page.',
      name: 'appIsSavedAsNotDeployed',
      desc: '',
      args: [appName],
    );
  }

  /// `There are no projects yet. Create one first.`
  String get appThereAreNoProjectsYet {
    return Intl.message(
      'There are no projects yet. Create one first.',
      name: 'appThereAreNoProjectsYet',
      desc: '',
      args: [],
    );
  }

  /// `The type is fixed once the app exists. Discard the app to choose another type.`
  String get appTheTypeIsFixedOnce {
    return Intl.message(
      'The type is fixed once the app exists. Discard the app to choose another type.',
      name: 'appTheTypeIsFixedOnce',
      desc: '',
      args: [],
    );
  }

  /// `Existing image`
  String get appExistingImage {
    return Intl.message(
      'Existing image',
      name: 'appExistingImage',
      desc: '',
      args: [],
    );
  }

  /// `Build from Git`
  String get appBuildFromGit {
    return Intl.message(
      'Build from Git',
      name: 'appBuildFromGit',
      desc: '',
      args: [],
    );
  }

  /// `ghcr.io/acme/shop:1.4.2`
  String get appGhcrIoAcmeShop1 {
    return Intl.message(
      'ghcr.io/acme/shop:1.4.2',
      name: 'appGhcrIoAcmeShop1',
      desc: '',
      args: [],
    );
  }

  /// `Any public image, or a private one with a login under Settings → Registries. Pin a version tag (not :latest) so deploys and rollbacks are reproducible.`
  String get appAnyPublicImageOrA {
    return Intl.message(
      'Any public image, or a private one with a login under Settings → Registries. Pin a version tag (not :latest) so deploys and rollbacks are reproducible.',
      name: 'appAnyPublicImageOrA',
      desc: '',
      args: [],
    );
  }

  /// `Shop API`
  String get appShopApi {
    return Intl.message('Shop API', name: 'appShopApi', desc: '', args: []);
  }

  /// `Used in Kubernetes names and the default domain`
  String get appUsedInKubernetesNamesAnd {
    return Intl.message(
      'Used in Kubernetes names and the default domain',
      name: 'appUsedInKubernetesNamesAnd',
      desc: '',
      args: [],
    );
  }

  /// `Port your app listens on`
  String get appPortYourAppListensOn {
    return Intl.message(
      'Port your app listens on',
      name: 'appPortYourAppListensOn',
      desc: '',
      args: [],
    );
  }

  /// `Also available to the app as $PORT`
  String get appAlsoAvailableToTheApp {
    return Intl.message(
      'Also available to the app as \$PORT',
      name: 'appAlsoAvailableToTheApp',
      desc: '',
      args: [],
    );
  }

  /// `No base domain is set, so the app gets no domain automatically. Set one in Settings, or add a domain on the app page later.`
  String get appNoBaseDomainIsSet {
    return Intl.message(
      'No base domain is set, so the app gets no domain automatically. Set one in Settings, or add a domain on the app page later.',
      name: 'appNoBaseDomainIsSet',
      desc: '',
      args: [],
    );
  }

  /// `default domain, with a Let's Encrypt certificate`
  String get appDefaultDomainWithALet {
    return Intl.message(
      'default domain, with a Let\'s Encrypt certificate',
      name: 'appDefaultDomainWithALet',
      desc: '',
      args: [],
    );
  }

  /// `Schedule (cron)`
  String get appScheduleCron {
    return Intl.message(
      'Schedule (cron)',
      name: 'appScheduleCron',
      desc: '',
      args: [],
    );
  }

  /// `e.g. */15 * * * * = every 15 minutes, @daily`
  String get appEG15Every15 {
    return Intl.message(
      'e.g. */15 * * * * = every 15 minutes, @daily',
      name: 'appEG15Every15',
      desc: '',
      args: [],
    );
  }

  /// `Cancel upload`
  String get appCancelUpload {
    return Intl.message(
      'Cancel upload',
      name: 'appCancelUpload',
      desc: '',
      args: [],
    );
  }

  /// `Retry upload`
  String get appRetryUpload {
    return Intl.message(
      'Retry upload',
      name: 'appRetryUpload',
      desc: '',
      args: [],
    );
  }

  /// `This file suits {label} better than the version you chose.`
  String appThisFileSuitsBetterThan(Object label) {
    return Intl.message(
      'This file suits $label better than the version you chose.',
      name: 'appThisFileSuitsBetterThan',
      desc: '',
      args: [label],
    );
  }

  /// `Use {label}`
  String appUse(Object label) {
    return Intl.message('Use $label', name: 'appUse', desc: '', args: [label]);
  }

  /// `CPU (millicores)`
  String get appCpuMillicores {
    return Intl.message(
      'CPU (millicores)',
      name: 'appCpuMillicores',
      desc: '',
      args: [],
    );
  }

  /// `Memory (MB)`
  String get appMemoryMb {
    return Intl.message('Memory (MB)', name: 'appMemoryMb', desc: '', args: []);
  }

  /// `Port open`
  String get appPortOpen {
    return Intl.message('Port open', name: 'appPortOpen', desc: '', args: []);
  }

  /// `HTTP`
  String get appHttp {
    return Intl.message('HTTP', name: 'appHttp', desc: '', args: []);
  }

  /// `None`
  String get appNone {
    return Intl.message('None', name: 'appNone', desc: '', args: []);
  }

  /// `Path`
  String get appPath {
    return Intl.message('Path', name: 'appPath', desc: '', args: []);
  }

  /// `Traffic only reaches a replica once its health check passes, so a broken release never replaces a working one.`
  String get appTrafficOnlyReachesAReplica {
    return Intl.message(
      'Traffic only reaches a replica once its health check passes, so a broken release never replaces a working one.',
      name: 'appTrafficOnlyReachesAReplica',
      desc: '',
      args: [],
    );
  }

  /// `--spring.profiles.active=prod`
  String get appSpringProfilesActiveProd {
    return Intl.message(
      '--spring.profiles.active=prod',
      name: 'appSpringProfilesActiveProd',
      desc: '',
      args: [],
    );
  }

  /// `Passed to your app as $APP_ARGS`
  String get appPassedToYourAppAs {
    return Intl.message(
      'Passed to your app as \$APP_ARGS',
      name: 'appPassedToYourAppAs',
      desc: '',
      args: [],
    );
  }

  /// `You can leave this page; the deploy continues on the server.`
  String get appYouCanLeaveThisPage {
    return Intl.message(
      'You can leave this page; the deploy continues on the server.',
      name: 'appYouCanLeaveThisPage',
      desc: '',
      args: [],
    );
  }

  /// `Back to the project`
  String get appBackToTheProject {
    return Intl.message(
      'Back to the project',
      name: 'appBackToTheProject',
      desc: '',
      args: [],
    );
  }

  /// `More replicas`
  String get appMoreReplicas {
    return Intl.message(
      'More replicas',
      name: 'appMoreReplicas',
      desc: '',
      args: [],
    );
  }

  /// `Changed {list} are saved but not live yet.`
  String appChangedAreSavedButNot(Object list) {
    return Intl.message(
      'Changed $list are saved but not live yet.',
      name: 'appChangedAreSavedButNot',
      desc: '',
      args: [list],
    );
  }

  /// `Redeploy`
  String get appRedeploy {
    return Intl.message('Redeploy', name: 'appRedeploy', desc: '', args: []);
  }

  /// `General`
  String get appGeneral {
    return Intl.message('General', name: 'appGeneral', desc: '', args: []);
  }

  /// `ghcr.io/acme/app:1.2.3`
  String get appGhcrIoAcmeApp1 {
    return Intl.message(
      'ghcr.io/acme/app:1.2.3',
      name: 'appGhcrIoAcmeApp1',
      desc: '',
      args: [],
    );
  }

  /// `Build from a Git repository instead`
  String get appBuildFromAGitRepository {
    return Intl.message(
      'Build from a Git repository instead',
      name: 'appBuildFromAGitRepository',
      desc: '',
      args: [],
    );
  }

  /// `Network and health check`
  String get appNetworkAndHealthCheck {
    return Intl.message(
      'Network and health check',
      name: 'appNetworkAndHealthCheck',
      desc: '',
      args: [],
    );
  }

  /// `Resources per replica`
  String get appResourcesPerReplica {
    return Intl.message(
      'Resources per replica',
      name: 'appResourcesPerReplica',
      desc: '',
      args: [],
    );
  }

  /// `No limit`
  String get appNoLimit {
    return Intl.message('No limit', name: 'appNoLimit', desc: '', args: []);
  }

  /// `Reserve these, but allow using any free capacity`
  String get appReserveTheseButAllowUsing {
    return Intl.message(
      'Reserve these, but allow using any free capacity',
      name: 'appReserveTheseButAllowUsing',
      desc: '',
      args: [],
    );
  }

  /// `Commands`
  String get appCommands {
    return Intl.message('Commands', name: 'appCommands', desc: '', args: []);
  }

  /// `Leave empty for the default`
  String get appLeaveEmptyForTheDefault {
    return Intl.message(
      'Leave empty for the default',
      name: 'appLeaveEmptyForTheDefault',
      desc: '',
      args: [],
    );
  }

  /// `e.g. ./migrate up`
  String get appEGMigrateUp {
    return Intl.message(
      'e.g. ./migrate up',
      name: 'appEGMigrateUp',
      desc: '',
      args: [],
    );
  }

  /// `Changing settings needs the app.edit permission.`
  String get appChangingSettingsNeedsTheApp {
    return Intl.message(
      'Changing settings needs the app.edit permission.',
      name: 'appChangingSettingsNeedsTheApp',
      desc: '',
      args: [],
    );
  }

  /// `Open ports on the server`
  String get appOpenPortsOnTheServer {
    return Intl.message(
      'Open ports on the server',
      name: 'appOpenPortsOnTheServer',
      desc: '',
      args: [],
    );
  }

  /// `TCP/UDP ports reachable from outside (MQTT, game servers…). Web traffic goes through domains instead.`
  String get appTcpUdpPortsReachableFrom {
    return Intl.message(
      'TCP/UDP ports reachable from outside (MQTT, game servers…). Web traffic goes through domains instead.',
      name: 'appTcpUdpPortsReachableFrom',
      desc: '',
      args: [],
    );
  }

  /// `Container port`
  String get appContainerPort {
    return Intl.message(
      'Container port',
      name: 'appContainerPort',
      desc: '',
      args: [],
    );
  }

  /// `TCP`
  String get appTcp {
    return Intl.message('TCP', name: 'appTcp', desc: '', args: []);
  }

  /// `UDP`
  String get appUdp {
    return Intl.message('UDP', name: 'appUdp', desc: '', args: []);
  }

  /// `Server port`
  String get appServerPort {
    return Intl.message(
      'Server port',
      name: 'appServerPort',
      desc: '',
      args: [],
    );
  }

  /// `auto (30000–32767)`
  String get appAuto3000032767 {
    return Intl.message(
      'auto (30000–32767)',
      name: 'appAuto3000032767',
      desc: '',
      args: [],
    );
  }

  /// `Open a port`
  String get appOpenAPort {
    return Intl.message(
      'Open a port',
      name: 'appOpenAPort',
      desc: '',
      args: [],
    );
  }

  /// `Persistent disks: files written there survive restarts and deploys. Everything else in the container is temporary.`
  String get appPersistentDisksFilesWrittenThere {
    return Intl.message(
      'Persistent disks: files written there survive restarts and deploys. Everything else in the container is temporary.',
      name: 'appPersistentDisksFilesWrittenThere',
      desc: '',
      args: [],
    );
  }

  /// `Mounted at`
  String get appMountedAt {
    return Intl.message('Mounted at', name: 'appMountedAt', desc: '', args: []);
  }

  /// `Size (MiB)`
  String get appSizeMib {
    return Intl.message('Size (MiB)', name: 'appSizeMib', desc: '', args: []);
  }

  /// `Remove from the app (the data is kept until you delete it)`
  String get appRemoveFromTheAppThe {
    return Intl.message(
      'Remove from the app (the data is kept until you delete it)',
      name: 'appRemoveFromTheAppThe',
      desc: '',
      args: [],
    );
  }

  /// `Used`
  String get appUsed {
    return Intl.message('Used', name: 'appUsed', desc: '', args: []);
  }

  /// `Files`
  String get appFiles {
    return Intl.message('Files', name: 'appFiles', desc: '', args: []);
  }

  /// `Add volume`
  String get appAddVolume {
    return Intl.message('Add volume', name: 'appAddVolume', desc: '', args: []);
  }

  /// `No volumes.`
  String get appNoVolumes {
    return Intl.message(
      'No volumes.',
      name: 'appNoVolumes',
      desc: '',
      args: [],
    );
  }

  /// `Removed from the app, data still kept`
  String get appRemovedFromTheAppData {
    return Intl.message(
      'Removed from the app, data still kept',
      name: 'appRemovedFromTheAppData',
      desc: '',
      args: [],
    );
  }

  /// `Delete data`
  String get appDeleteData {
    return Intl.message(
      'Delete data',
      name: 'appDeleteData',
      desc: '',
      args: [],
    );
  }

  /// `Storage`
  String get appStorage {
    return Intl.message('Storage', name: 'appStorage', desc: '', args: []);
  }

  /// `Default`
  String get appDefault {
    return Intl.message('Default', name: 'appDefault', desc: '', args: []);
  }

  /// `Shared by all replicas`
  String get appSharedByAllReplicas {
    return Intl.message(
      'Shared by all replicas',
      name: 'appSharedByAllReplicas',
      desc: '',
      args: [],
    );
  }

  /// `Fixed once the volume exists.`
  String get appFixedOnceTheVolumeExists {
    return Intl.message(
      'Fixed once the volume exists.',
      name: 'appFixedOnceTheVolumeExists',
      desc: '',
      args: [],
    );
  }

  /// `Delete the data of {vName}?`
  String appDeleteTheDataOf(Object vName) {
    return Intl.message(
      'Delete the data of $vName?',
      name: 'appDeleteTheDataOf',
      desc: '',
      args: [vName],
    );
  }

  /// `Everything stored on this volume is deleted for good.`
  String get appEverythingStoredOnThisVolume {
    return Intl.message(
      'Everything stored on this volume is deleted for good.',
      name: 'appEverythingStoredOnThisVolume',
      desc: '',
      args: [],
    );
  }

  /// `Volume {vName} deleted`
  String appVolumeDeleted(Object vName) {
    return Intl.message(
      'Volume $vName deleted',
      name: 'appVolumeDeleted',
      desc: '',
      args: [vName],
    );
  }

  /// `Newest pod`
  String get appNewestPod {
    return Intl.message('Newest pod', name: 'appNewestPod', desc: '', args: []);
  }

  /// `Disconnect`
  String get appDisconnect {
    return Intl.message(
      'Disconnect',
      name: 'appDisconnect',
      desc: '',
      args: [],
    );
  }

  /// `Changes inside the container are lost on restart, except on volumes. Every session is recorded in the audit log.`
  String get appChangesInsideTheContainerAre {
    return Intl.message(
      'Changes inside the container are lost on restart, except on volumes. Every session is recorded in the audit log.',
      name: 'appChangesInsideTheContainerAre',
      desc: '',
      args: [],
    );
  }

  /// `Files up to 64 MB can be uploaded here`
  String get appFilesUpTo64Mb {
    return Intl.message(
      'Files up to 64 MB can be uploaded here',
      name: 'appFilesUpTo64Mb',
      desc: '',
      args: [],
    );
  }

  /// `Replace {fName}?`
  String appReplace(Object fName) {
    return Intl.message(
      'Replace $fName?',
      name: 'appReplace',
      desc: '',
      args: [fName],
    );
  }

  /// `The file in this folder is overwritten.`
  String get appTheFileInThisFolder {
    return Intl.message(
      'The file in this folder is overwritten.',
      name: 'appTheFileInThisFolder',
      desc: '',
      args: [],
    );
  }

  /// `Replace`
  String get appReplace2 {
    return Intl.message('Replace', name: 'appReplace2', desc: '', args: []);
  }

  /// `Saved {eName}`
  String appSaved(Object eName) {
    return Intl.message(
      'Saved $eName',
      name: 'appSaved',
      desc: '',
      args: [eName],
    );
  }

  /// `{eName} is not a text file — download it instead`
  String appIsNotATextFile(Object eName) {
    return Intl.message(
      '$eName is not a text file — download it instead',
      name: 'appIsNotATextFile',
      desc: '',
      args: [eName],
    );
  }

  /// `Delete {what}?`
  String appDelete2(Object what) {
    return Intl.message(
      'Delete $what?',
      name: 'appDelete2',
      desc: '',
      args: [what],
    );
  }

  /// `This cannot be undone (backups of the volume keep older copies).`
  String get appThisCannotBeUndoneBackups {
    return Intl.message(
      'This cannot be undone (backups of the volume keep older copies).',
      name: 'appThisCannotBeUndoneBackups',
      desc: '',
      args: [],
    );
  }

  /// `Files · {volume}`
  String appFiles2(Object volume) {
    return Intl.message(
      'Files · $volume',
      name: 'appFiles2',
      desc: '',
      args: [volume],
    );
  }

  /// `Upload`
  String get appUpload {
    return Intl.message('Upload', name: 'appUpload', desc: '', args: []);
  }

  /// `New folder`
  String get appNewFolder {
    return Intl.message('New folder', name: 'appNewFolder', desc: '', args: []);
  }

  /// `Changes apply to the running app at once. New files get the owner of their folder.`
  String get appChangesApplyToTheRunning {
    return Intl.message(
      'Changes apply to the running app at once. New files get the owner of their folder.',
      name: 'appChangesApplyToTheRunning',
      desc: '',
      args: [],
    );
  }

  /// `Opening the volume…`
  String get appOpeningTheVolume {
    return Intl.message(
      'Opening the volume…',
      name: 'appOpeningTheVolume',
      desc: '',
      args: [],
    );
  }

  /// `Empty folder`
  String get appEmptyFolder {
    return Intl.message(
      'Empty folder',
      name: 'appEmptyFolder',
      desc: '',
      args: [],
    );
  }

  /// `Download`
  String get appDownload {
    return Intl.message('Download', name: 'appDownload', desc: '', args: []);
  }

  /// `Rename / move`
  String get appRenameMove {
    return Intl.message(
      'Rename / move',
      name: 'appRenameMove',
      desc: '',
      args: [],
    );
  }

  /// `/new or https://…`
  String get appNewOrHttps {
    return Intl.message(
      '/new or https://…',
      name: 'appNewOrHttps',
      desc: '',
      args: [],
    );
  }

  /// `Also redirect /from/… and keep the rest of the path`
  String get appAlsoRedirectFromAndKeep {
    return Intl.message(
      'Also redirect /from/… and keep the rest of the path',
      name: 'appAlsoRedirectFromAndKeep',
      desc: '',
      args: [],
    );
  }

  /// `Remove rule`
  String get appRemoveRule {
    return Intl.message(
      'Remove rule',
      name: 'appRemoveRule',
      desc: '',
      args: [],
    );
  }

  /// `Add redirect rule`
  String get appAddRedirectRule {
    return Intl.message(
      'Add redirect rule',
      name: 'appAddRedirectRule',
      desc: '',
      args: [],
    );
  }

  /// `Allowed IPs`
  String get appAllowedIps {
    return Intl.message(
      'Allowed IPs',
      name: 'appAllowedIps',
      desc: '',
      args: [],
    );
  }

  /// `One IP or CIDR per line; everyone else gets 403. Empty = open to everyone.`
  String get appOneIpOrCidrPer {
    return Intl.message(
      'One IP or CIDR per line; everyone else gets 403. Empty = open to everyone.',
      name: 'appOneIpOrCidrPer',
      desc: '',
      args: [],
    );
  }

  /// `Response headers`
  String get appResponseHeaders {
    return Intl.message(
      'Response headers',
      name: 'appResponseHeaders',
      desc: '',
      args: [],
    );
  }

  /// `X-Frame-Options: DENY`
  String get appXFrameOptionsDeny {
    return Intl.message(
      'X-Frame-Options: DENY',
      name: 'appXFrameOptionsDeny',
      desc: '',
      args: [],
    );
  }

  /// `Request headers (to the app)`
  String get appRequestHeadersToTheApp {
    return Intl.message(
      'Request headers (to the app)',
      name: 'appRequestHeadersToTheApp',
      desc: '',
      args: [],
    );
  }

  /// `X-Env: staging`
  String get appXEnvStaging {
    return Intl.message(
      'X-Env: staging',
      name: 'appXEnvStaging',
      desc: '',
      args: [],
    );
  }

  /// `"Name: value" per line; an empty value removes the header.`
  String get appNameValuePerLineAn {
    return Intl.message(
      '"Name: value" per line; an empty value removes the header.',
      name: 'appNameValuePerLineAn',
      desc: '',
      args: [],
    );
  }

  /// `Let other sites call this domain from the browser`
  String get appLetOtherSitesCallThis {
    return Intl.message(
      'Let other sites call this domain from the browser',
      name: 'appLetOtherSitesCallThis',
      desc: '',
      args: [],
    );
  }

  /// `Allowed origins`
  String get appAllowedOrigins {
    return Intl.message(
      'Allowed origins',
      name: 'appAllowedOrigins',
      desc: '',
      args: [],
    );
  }

  /// `One per line, or * for any site (then without credentials).`
  String get appOnePerLineOrFor {
    return Intl.message(
      'One per line, or * for any site (then without credentials).',
      name: 'appOnePerLineOrFor',
      desc: '',
      args: [],
    );
  }

  /// `Allow cookies / credentials`
  String get appAllowCookiesCredentials {
    return Intl.message(
      'Allow cookies / credentials',
      name: 'appAllowCookiesCredentials',
      desc: '',
      args: [],
    );
  }

  /// `Compress text responses (gzip)`
  String get appCompressTextResponsesGzip {
    return Intl.message(
      'Compress text responses (gzip)',
      name: 'appCompressTextResponsesGzip',
      desc: '',
      args: [],
    );
  }

  /// `HTML, CSS, JS, JSON … that the app does not compress itself`
  String get appHtmlCssJsJsonThat {
    return Intl.message(
      'HTML, CSS, JS, JSON … that the app does not compress itself',
      name: 'appHtmlCssJsJsonThat',
      desc: '',
      args: [],
    );
  }

  /// `HSTS (browsers only use HTTPS)`
  String get appHstsBrowsersOnlyUseHttps {
    return Intl.message(
      'HSTS (browsers only use HTTPS)',
      name: 'appHstsBrowsersOnlyUseHttps',
      desc: '',
      args: [],
    );
  }

  /// `Off`
  String get appOff {
    return Intl.message('Off', name: 'appOff', desc: '', args: []);
  }

  /// `5 minutes (testing)`
  String get app5MinutesTesting {
    return Intl.message(
      '5 minutes (testing)',
      name: 'app5MinutesTesting',
      desc: '',
      args: [],
    );
  }

  /// `6 months`
  String get app6Months {
    return Intl.message('6 months', name: 'app6Months', desc: '', args: []);
  }

  /// `1 year`
  String get app1Year {
    return Intl.message('1 year', name: 'app1Year', desc: '', args: []);
  }

  /// `2 years`
  String get app2Years {
    return Intl.message('2 years', name: 'app2Years', desc: '', args: []);
  }

  /// `Include subdomains`
  String get appIncludeSubdomains {
    return Intl.message(
      'Include subdomains',
      name: 'appIncludeSubdomains',
      desc: '',
      args: [],
    );
  }

  /// `Only an Owner can set up the panel`
  String get onboardingOnlyAnOwnerCanSet {
    return Intl.message(
      'Only an Owner can set up the panel',
      name: 'onboardingOnlyAnOwnerCanSet',
      desc: '',
      args: [],
    );
  }

  /// `Set up Nectar Apps`
  String get onboardingSetUpNectarApps {
    return Intl.message(
      'Set up Nectar Apps',
      name: 'onboardingSetUpNectarApps',
      desc: '',
      args: [],
    );
  }

  /// `Three short steps. You can change everything later in Settings.`
  String get onboardingThreeShortStepsYouCan {
    return Intl.message(
      'Three short steps. You can change everything later in Settings.',
      name: 'onboardingThreeShortStepsYouCan',
      desc: '',
      args: [],
    );
  }

  /// `Skip setup for now`
  String get onboardingSkipSetupForNow {
    return Intl.message(
      'Skip setup for now',
      name: 'onboardingSkipSetupForNow',
      desc: '',
      args: [],
    );
  }

  /// `Base domain`
  String get onboardingBaseDomain {
    return Intl.message(
      'Base domain',
      name: 'onboardingBaseDomain',
      desc: '',
      args: [],
    );
  }

  /// `At your DNS provider, add one record:`
  String get onboardingAtYourDnsProviderAdd {
    return Intl.message(
      'At your DNS provider, add one record:',
      name: 'onboardingAtYourDnsProviderAdd',
      desc: '',
      args: [],
    );
  }

  /// `Continue`
  String get onboardingContinue {
    return Intl.message(
      'Continue',
      name: 'onboardingContinue',
      desc: '',
      args: [],
    );
  }

  /// `Email for Let's Encrypt`
  String get onboardingEmailForLetSEncrypt {
    return Intl.message(
      'Email for Let\'s Encrypt',
      name: 'onboardingEmailForLetSEncrypt',
      desc: '',
      args: [],
    );
  }

  /// `Only used for expiry warnings`
  String get onboardingOnlyUsedForExpiryWarnings {
    return Intl.message(
      'Only used for expiry warnings',
      name: 'onboardingOnlyUsedForExpiryWarnings',
      desc: '',
      args: [],
    );
  }

  /// `Leave it empty to register without an email.`
  String get onboardingLeaveItEmptyToRegister {
    return Intl.message(
      'Leave it empty to register without an email.',
      name: 'onboardingLeaveItEmptyToRegister',
      desc: '',
      args: [],
    );
  }

  /// `Image: {sampleImage} · 50m CPU · 64 MB`
  String onboardingImage50mCpu64Mb(Object sampleImage) {
    return Intl.message(
      'Image: $sampleImage · 50m CPU · 64 MB',
      name: 'onboardingImage50mCpu64Mb',
      desc: '',
      args: [sampleImage],
    );
  }

  /// `Skip`
  String get onboardingSkip {
    return Intl.message('Skip', name: 'onboardingSkip', desc: '', args: []);
  }

  /// `Deploy sample`
  String get onboardingDeploySample {
    return Intl.message(
      'Deploy sample',
      name: 'onboardingDeploySample',
      desc: '',
      args: [],
    );
  }

  /// `All alerts`
  String get alertAllAlerts {
    return Intl.message(
      'All alerts',
      name: 'alertAllAlerts',
      desc: '',
      args: [],
    );
  }

  /// `and {alertsLength} more`
  String alertAndMore(Object alertsLength) {
    return Intl.message(
      'and $alertsLength more',
      name: 'alertAndMore',
      desc: '',
      args: [alertsLength],
    );
  }

  /// `Get notified when apps go down, servers run out of resources or certificates are about to expire.`
  String get alertGetNotifiedWhenAppsGo {
    return Intl.message(
      'Get notified when apps go down, servers run out of resources or certificates are about to expire.',
      name: 'alertGetNotifiedWhenAppsGo',
      desc: '',
      args: [],
    );
  }

  /// `No notification channel yet: alerts only show up here. Add ntfy, Telegram, email or a webhook under Channels.`
  String get alertNoNotificationChannelYetAlerts {
    return Intl.message(
      'No notification channel yet: alerts only show up here. Add ntfy, Telegram, email or a webhook under Channels.',
      name: 'alertNoNotificationChannelYetAlerts',
      desc: '',
      args: [],
    );
  }

  /// `{title}: enter a number`
  String alertEnterANumber(Object title) {
    return Intl.message(
      '$title: enter a number',
      name: 'alertEnterANumber',
      desc: '',
      args: [title],
    );
  }

  /// `Alert rules saved`
  String get alertAlertRulesSaved {
    return Intl.message(
      'Alert rules saved',
      name: 'alertAlertRulesSaved',
      desc: '',
      args: [],
    );
  }

  /// `Notifications`
  String get alertNotifications {
    return Intl.message(
      'Notifications',
      name: 'alertNotifications',
      desc: '',
      args: [],
    );
  }

  /// `Panel address for links`
  String get alertPanelAddressForLinks {
    return Intl.message(
      'Panel address for links',
      name: 'alertPanelAddressForLinks',
      desc: '',
      args: [],
    );
  }

  /// `Notifications link to the affected app. Leave empty for no links.`
  String get alertNotificationsLinkToTheAffected {
    return Intl.message(
      'Notifications link to the affected app. Leave empty for no links.',
      name: 'alertNotificationsLinkToTheAffected',
      desc: '',
      args: [],
    );
  }

  /// `Remind me while an alert keeps firing`
  String get alertRemindMeWhileAnAlert {
    return Intl.message(
      'Remind me while an alert keeps firing',
      name: 'alertRemindMeWhileAnAlert',
      desc: '',
      args: [],
    );
  }

  /// `Changing rules needs the settings.manage permission.`
  String get alertChangingRulesNeedsTheSettings {
    return Intl.message(
      'Changing rules needs the settings.manage permission.',
      name: 'alertChangingRulesNeedsTheSettings',
      desc: '',
      args: [],
    );
  }

  /// `For at least`
  String get alertForAtLeast {
    return Intl.message(
      'For at least',
      name: 'alertForAtLeast',
      desc: '',
      args: [],
    );
  }

  /// `Test message sent to {chName}`
  String alertTestMessageSentTo(Object chName) {
    return Intl.message(
      'Test message sent to $chName',
      name: 'alertTestMessageSentTo',
      desc: '',
      args: [chName],
    );
  }

  /// `Delete {chName}?`
  String alertDelete(Object chName) {
    return Intl.message(
      'Delete $chName?',
      name: 'alertDelete',
      desc: '',
      args: [chName],
    );
  }

  /// `Alerts will no longer be sent there.`
  String get alertAlertsWillNoLongerBe {
    return Intl.message(
      'Alerts will no longer be sent there.',
      name: 'alertAlertsWillNoLongerBe',
      desc: '',
      args: [],
    );
  }

  /// `Channels`
  String get alertChannels {
    return Intl.message('Channels', name: 'alertChannels', desc: '', args: []);
  }

  /// `Add channel`
  String get alertAddChannel {
    return Intl.message(
      'Add channel',
      name: 'alertAddChannel',
      desc: '',
      args: [],
    );
  }

  /// `No channels yet`
  String get alertNoChannelsYet {
    return Intl.message(
      'No channels yet',
      name: 'alertNoChannelsYet',
      desc: '',
      args: [],
    );
  }

  /// `ntfy is the quickest: install the ntfy app, subscribe to a topic name only you know, and add it here.`
  String get alertNtfyIsTheQuickestInstall {
    return Intl.message(
      'ntfy is the quickest: install the ntfy app, subscribe to a topic name only you know, and add it here.',
      name: 'alertNtfyIsTheQuickestInstall',
      desc: '',
      args: [],
    );
  }

  /// `Send a test`
  String get alertSendATest {
    return Intl.message(
      'Send a test',
      name: 'alertSendATest',
      desc: '',
      args: [],
    );
  }

  /// `Send with the panel's email settings`
  String get alertSendWithThePanelS {
    return Intl.message(
      'Send with the panel\'s email settings',
      name: 'alertSendWithThePanelS',
      desc: '',
      args: [],
    );
  }

  /// `Settings → Email; turn off to use another SMTP server`
  String get alertSettingsEmailTurnOffTo {
    return Intl.message(
      'Settings → Email; turn off to use another SMTP server',
      name: 'alertSettingsEmailTurnOffTo',
      desc: '',
      args: [],
    );
  }

  /// `Encryption`
  String get alertEncryption {
    return Intl.message(
      'Encryption',
      name: 'alertEncryption',
      desc: '',
      args: [],
    );
  }

  /// `STARTTLS (port 587)`
  String get alertStarttlsPort587 {
    return Intl.message(
      'STARTTLS (port 587)',
      name: 'alertStarttlsPort587',
      desc: '',
      args: [],
    );
  }

  /// `TLS (port 465)`
  String get alertTlsPort465 {
    return Intl.message(
      'TLS (port 465)',
      name: 'alertTlsPort465',
      desc: '',
      args: [],
    );
  }

  /// `None (port 25, local relays only)`
  String get alertNonePort25LocalRelays {
    return Intl.message(
      'None (port 25, local relays only)',
      name: 'alertNonePort25LocalRelays',
      desc: '',
      args: [],
    );
  }

  /// `Finish setting up: domain, HTTPS email and a sample app.`
  String get dashboardFinishSettingUpDomainHttps {
    return Intl.message(
      'Finish setting up: domain, HTTPS email and a sample app.',
      name: 'dashboardFinishSettingUpDomainHttps',
      desc: '',
      args: [],
    );
  }

  /// `Continue setup`
  String get dashboardContinueSetup {
    return Intl.message(
      'Continue setup',
      name: 'dashboardContinueSetup',
      desc: '',
      args: [],
    );
  }

  /// `Need attention`
  String get dashboardNeedAttention {
    return Intl.message(
      'Need attention',
      name: 'dashboardNeedAttention',
      desc: '',
      args: [],
    );
  }

  /// `No apps yet.`
  String get dashboardNoAppsYet {
    return Intl.message(
      'No apps yet.',
      name: 'dashboardNoAppsYet',
      desc: '',
      args: [],
    );
  }

  /// `Nothing deployed yet`
  String get dashboardNothingDeployedYet {
    return Intl.message(
      'Nothing deployed yet',
      name: 'dashboardNothingDeployedYet',
      desc: '',
      args: [],
    );
  }

  /// `A project is a space for related apps with its own quota. Create one, then add your first app.`
  String get dashboardAProjectIsASpace {
    return Intl.message(
      'A project is a space for related apps with its own quota. Create one, then add your first app.',
      name: 'dashboardAProjectIsASpace',
      desc: '',
      args: [],
    );
  }

  /// `Later`
  String get shellLater {
    return Intl.message('Later', name: 'shellLater', desc: '', args: []);
  }

  /// `Reload`
  String get shellReload {
    return Intl.message('Reload', name: 'shellReload', desc: '', args: []);
  }

  /// `Nothing yet.`
  String get widgetNothingYet {
    return Intl.message(
      'Nothing yet.',
      name: 'widgetNothingYet',
      desc: '',
      args: [],
    );
  }

  /// `No data yet: history fills in every 15 seconds.`
  String get widgetNoDataYetHistoryFills {
    return Intl.message(
      'No data yet: history fills in every 15 seconds.',
      name: 'widgetNoDataYetHistoryFills',
      desc: '',
      args: [],
    );
  }

  /// `Retry`
  String get widgetRetry {
    return Intl.message('Retry', name: 'widgetRetry', desc: '', args: []);
  }

  /// `Loading`
  String get widgetLoading {
    return Intl.message('Loading', name: 'widgetLoading', desc: '', args: []);
  }

  /// `Preview environments`
  String get appPreviewEnvironments {
    return Intl.message(
      'Preview environments',
      name: 'appPreviewEnvironments',
      desc: '',
      args: [],
    );
  }

  /// `A preview for every pull request`
  String get appPreviewsSwitch {
    return Intl.message(
      'A preview for every pull request',
      name: 'appPreviewsSwitch',
      desc: '',
      args: [],
    );
  }

  /// `Each pull request into this branch gets its own copy of the app at its own domain, rebuilt on new commits and deleted when the pull request closes. Needs the webhook with pull request events.`
  String get appPreviewsHelp {
    return Intl.message(
      'Each pull request into this branch gets its own copy of the app at its own domain, rebuilt on new commits and deleted when the pull request closes. Needs the webhook with pull request events.',
      name: 'appPreviewsHelp',
      desc: '',
      args: [],
    );
  }

  /// `Previews use this app's databases`
  String get appPreviewDatabases {
    return Intl.message(
      'Previews use this app\'s databases',
      name: 'appPreviewDatabases',
      desc: '',
      args: [],
    );
  }

  /// `Off: previews start without database links (they must not change real data).`
  String get appPreviewDatabasesHelp {
    return Intl.message(
      'Off: previews start without database links (they must not change real data).',
      name: 'appPreviewDatabasesHelp',
      desc: '',
      args: [],
    );
  }

  /// `Also pull requests from forks`
  String get appPreviewForks {
    return Intl.message(
      'Also pull requests from forks',
      name: 'appPreviewForks',
      desc: '',
      args: [],
    );
  }

  /// `Their code runs with this app's variables and secrets.`
  String get appPreviewForksHelp {
    return Intl.message(
      'Their code runs with this app\'s variables and secrets.',
      name: 'appPreviewForksHelp',
      desc: '',
      args: [],
    );
  }

  /// `Delete inactive previews after`
  String get appPreviewTtl {
    return Intl.message(
      'Delete inactive previews after',
      name: 'appPreviewTtl',
      desc: '',
      args: [],
    );
  }

  /// `never`
  String get appPreviewTtlNever {
    return Intl.message(
      'never',
      name: 'appPreviewTtlNever',
      desc: '',
      args: [],
    );
  }

  /// `{days} days`
  String appPreviewDays(Object days) {
    return Intl.message(
      '$days days',
      name: 'appPreviewDays',
      desc: '',
      args: [days],
    );
  }

  /// `Previews`
  String get appPreviews {
    return Intl.message('Previews', name: 'appPreviews', desc: '', args: []);
  }

  /// `No open pull request has a preview.`
  String get appNoPreviews {
    return Intl.message(
      'No open pull request has a preview.',
      name: 'appNoPreviews',
      desc: '',
      args: [],
    );
  }

  /// `Preview of {parent} · pull request #{number}`
  String appPreviewOf(Object parent, Object number) {
    return Intl.message(
      'Preview of $parent · pull request #$number',
      name: 'appPreviewOf',
      desc: '',
      args: [parent, number],
    );
  }

  /// `Rebuilt on new commits and deleted when the pull request closes. Change settings on {parent}.`
  String appPreviewOfHelp(Object parent) {
    return Intl.message(
      'Rebuilt on new commits and deleted when the pull request closes. Change settings on $parent.',
      name: 'appPreviewOfHelp',
      desc: '',
      args: [parent],
    );
  }

  /// `Pull request`
  String get appOpenPullRequest {
    return Intl.message(
      'Pull request',
      name: 'appOpenPullRequest',
      desc: '',
      args: [],
    );
  }

  /// `Open`
  String get appOpenPreview {
    return Intl.message('Open', name: 'appOpenPreview', desc: '', args: []);
  }

  /// `Delete the preview {slug}?`
  String appDeletePreview(Object slug) {
    return Intl.message(
      'Delete the preview $slug?',
      name: 'appDeletePreview',
      desc: '',
      args: [slug],
    );
  }

  /// `A new commit to the pull request creates it again.`
  String get appDeletePreviewHelp {
    return Intl.message(
      'A new commit to the pull request creates it again.',
      name: 'appDeletePreviewHelp',
      desc: '',
      args: [],
    );
  }

  /// `or`
  String get authOr {
    return Intl.message('or', name: 'authOr', desc: '', args: []);
  }

  /// `Sign in with a passkey`
  String get authSignInWithPasskey {
    return Intl.message(
      'Sign in with a passkey',
      name: 'authSignInWithPasskey',
      desc: '',
      args: [],
    );
  }

  /// `Passkeys`
  String get settingPasskeys {
    return Intl.message(
      'Passkeys',
      name: 'settingPasskeys',
      desc: '',
      args: [],
    );
  }

  /// `Add passkey`
  String get settingAddPasskey {
    return Intl.message(
      'Add passkey',
      name: 'settingAddPasskey',
      desc: '',
      args: [],
    );
  }

  /// `New passkey`
  String get settingPasskeyName {
    return Intl.message(
      'New passkey',
      name: 'settingPasskeyName',
      desc: '',
      args: [],
    );
  }

  /// `Name (e.g. MacBook, iPhone, YubiKey)`
  String get settingPasskeyNameHint {
    return Intl.message(
      'Name (e.g. MacBook, iPhone, YubiKey)',
      name: 'settingPasskeyNameHint',
      desc: '',
      args: [],
    );
  }

  /// `Passkey added: sign in with it on the login page`
  String get settingPasskeyAdded {
    return Intl.message(
      'Passkey added: sign in with it on the login page',
      name: 'settingPasskeyAdded',
      desc: '',
      args: [],
    );
  }

  /// `Remove the passkey {name}?`
  String settingPasskeyDelete(Object name) {
    return Intl.message(
      'Remove the passkey $name?',
      name: 'settingPasskeyDelete',
      desc: '',
      args: [name],
    );
  }

  /// `It can no longer sign in here. Also delete it from your device or password manager.`
  String get settingPasskeyDeleteHelp {
    return Intl.message(
      'It can no longer sign in here. Also delete it from your device or password manager.',
      name: 'settingPasskeyDeleteHelp',
      desc: '',
      args: [],
    );
  }

  /// `Sign in with Touch ID, Face ID, Windows Hello, your phone or a security key — no password and no code needed. A passkey works at this panel address only.`
  String get settingPasskeysHelp {
    return Intl.message(
      'Sign in with Touch ID, Face ID, Windows Hello, your phone or a security key — no password and no code needed. A passkey works at this panel address only.',
      name: 'settingPasskeysHelp',
      desc: '',
      args: [],
    );
  }

  /// `synced`
  String get settingPasskeySynced {
    return Intl.message(
      'synced',
      name: 'settingPasskeySynced',
      desc: '',
      args: [],
    );
  }

  /// `not used yet`
  String get settingPasskeyNeverUsed {
    return Intl.message(
      'not used yet',
      name: 'settingPasskeyNeverUsed',
      desc: '',
      args: [],
    );
  }

  /// `used {when}`
  String settingPasskeyUsed(Object when) {
    return Intl.message(
      'used $when',
      name: 'settingPasskeyUsed',
      desc: '',
      args: [when],
    );
  }

  /// `Add and use passkeys in the web panel (a recent Chrome, Edge, Safari or Firefox).`
  String get settingPasskeysWebOnly {
    return Intl.message(
      'Add and use passkeys in the web panel (a recent Chrome, Edge, Safari or Firefox).',
      name: 'settingPasskeysWebOnly',
      desc: '',
      args: [],
    );
  }

  /// `The server's clock is {amount} behind this device: times in the panel look wrong, and two-factor codes and certificates can fail. On the server run: sudo timedatectl set-ntp true`
  String shellClockBehind(Object amount) {
    return Intl.message(
      'The server\'s clock is $amount behind this device: times in the panel look wrong, and two-factor codes and certificates can fail. On the server run: sudo timedatectl set-ntp true',
      name: 'shellClockBehind',
      desc: '',
      args: [amount],
    );
  }

  /// `The server's clock is {amount} ahead of this device: times in the panel look wrong, and two-factor codes and certificates can fail. On the server run: sudo timedatectl set-ntp true`
  String shellClockAhead(Object amount) {
    return Intl.message(
      'The server\'s clock is $amount ahead of this device: times in the panel look wrong, and two-factor codes and certificates can fail. On the server run: sudo timedatectl set-ntp true',
      name: 'shellClockAhead',
      desc: '',
      args: [amount],
    );
  }

  /// `Mark as read`
  String get alertMarkRead {
    return Intl.message(
      'Mark as read',
      name: 'alertMarkRead',
      desc: '',
      args: [],
    );
  }

  /// `Mark as unread`
  String get alertMarkUnread {
    return Intl.message(
      'Mark as unread',
      name: 'alertMarkUnread',
      desc: '',
      args: [],
    );
  }

  /// `Mark all read`
  String get alertMarkAllRead {
    return Intl.message(
      'Mark all read',
      name: 'alertMarkAllRead',
      desc: '',
      args: [],
    );
  }

  /// `read`
  String get alertReadLabel {
    return Intl.message('read', name: 'alertReadLabel', desc: '', args: []);
  }

  /// `Overview`
  String get addonTabOverview {
    return Intl.message(
      'Overview',
      name: 'addonTabOverview',
      desc: '',
      args: [],
    );
  }

  /// `Metrics`
  String get addonTabMetrics {
    return Intl.message('Metrics', name: 'addonTabMetrics', desc: '', args: []);
  }

  /// `Logs`
  String get addonTabLogs {
    return Intl.message('Logs', name: 'addonTabLogs', desc: '', args: []);
  }

  /// `Terminal`
  String get addonTabTerminal {
    return Intl.message(
      'Terminal',
      name: 'addonTabTerminal',
      desc: '',
      args: [],
    );
  }

  /// `Events`
  String get addonTabEvents {
    return Intl.message('Events', name: 'addonTabEvents', desc: '', args: []);
  }

  /// `Share with the whole project`
  String get addonShareWithProject {
    return Intl.message(
      'Share with the whole project',
      name: 'addonShareWithProject',
      desc: '',
      args: [],
    );
  }

  /// `Every app in the project gets the connection variables, including apps you add later. No link needed. Redeploy the apps to apply.`
  String get addonShareHelp {
    return Intl.message(
      'Every app in the project gets the connection variables, including apps you add later. No link needed. Redeploy the apps to apply.',
      name: 'addonShareHelp',
      desc: '',
      args: [],
    );
  }

  /// `All apps get: {vars}`
  String addonSharedAs(String vars) {
    return Intl.message(
      'All apps get: $vars',
      name: 'addonSharedAs',
      desc: '',
      args: [vars],
    );
  }

  /// `Saved. Redeploy the apps to apply.`
  String get addonSharedSaved {
    return Intl.message(
      'Saved. Redeploy the apps to apply.',
      name: 'addonSharedSaved',
      desc: '',
      args: [],
    );
  }

  /// `Prefix for the shared variables`
  String get addonSharePrefix {
    return Intl.message(
      'Prefix for the shared variables',
      name: 'addonSharePrefix',
      desc: '',
      args: [],
    );
  }

  /// `An app linked here uses its own link (and prefix) instead.`
  String get addonLinkOverridesShared {
    return Intl.message(
      'An app linked here uses its own link (and prefix) instead.',
      name: 'addonLinkOverridesShared',
      desc: '',
      args: [],
    );
  }

  /// `shared with the project`
  String get appSharedWithProject {
    return Intl.message(
      'shared with the project',
      name: 'appSharedWithProject',
      desc: '',
      args: [],
    );
  }

  /// `Open the database`
  String get projectOpenTheDatabase {
    return Intl.message(
      'Open the database',
      name: 'projectOpenTheDatabase',
      desc: '',
      args: [],
    );
  }

  /// `Search: MQTT, Kafka, Redis, search…`
  String get projectSearchTemplates {
    return Intl.message(
      'Search: MQTT, Kafka, Redis, search…',
      name: 'projectSearchTemplates',
      desc: '',
      args: [],
    );
  }

  /// `All`
  String get projectAllTemplates {
    return Intl.message('All', name: 'projectAllTemplates', desc: '', args: []);
  }
}

class AppLocalizationDelegate extends LocalizationsDelegate<S> {
  const AppLocalizationDelegate();

  List<Locale> get supportedLocales {
    return const <Locale>[
      Locale.fromSubtags(languageCode: 'en'),
      Locale.fromSubtags(languageCode: 'uk'),
    ];
  }

  @override
  bool isSupported(Locale locale) => _isSupported(locale);
  @override
  Future<S> load(Locale locale) => S.load(locale);
  @override
  bool shouldReload(AppLocalizationDelegate old) => false;

  bool _isSupported(Locale locale) {
    for (var supportedLocale in supportedLocales) {
      if (supportedLocale.languageCode == locale.languageCode) {
        return true;
      }
    }
    return false;
  }
}

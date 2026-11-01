import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/copy.dart';
import '../../widgets/status.dart';

String buildTone(String s) => switch (s) {
  'succeeded' => 'running',
  'running' => 'deploying',
  'canceled' => 'stopped',
  _ => 'failed',
};

/// Builds: the Git source (repository, branch, webhook) and the build history with logs.
/// GET /apps/{id}/previews: the app's preview environments, or what this preview app previews.
final previewsProvider = FutureProvider.autoDispose
    .family<({List<PreviewInfo> previews, PreviewInfo? of}), int>((
      ref,
      appId,
    ) async {
      final v = await ref.watch(apiProvider).get('/apps/$appId/previews');
      final m = v is Map ? v : const {};
      return (
        previews: [
          for (final p in (m['previews'] as List?) ?? const [])
            PreviewInfo.fromJson(p),
        ],
        of: m['preview_of'] == null
            ? null
            : PreviewInfo.fromJson(m['preview_of']),
      );
    });

class BuildsTab extends ConsumerWidget {
  const BuildsTab({super.key, required this.app, required this.source});

  final App app;
  final AppSourceInfo source;

  Future<void> _build(BuildContext context, WidgetRef ref) async {
    try {
      final res = await ref
          .read(apiProvider)
          .post('/apps/${app.id}/builds', data: {'deploy': true});
      ref.invalidate(buildsProvider(app.id));
      if (context.mounted) {
        showSnack(context, 'Build #${(res as Map)['number']} started');
      }
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final session = ref.watch(sessionProvider);
    final canBuild = session.can('app.deploy', projectId: app.projectId);
    final canEdit = session.can('app.edit', projectId: app.projectId);
    final builds = ref.watch(buildsProvider(app.id));
    final c = NectarColors.of(context);
    final busy = builds.value?.any((b) => b.running) ?? false;
    final previews = ref.watch(previewsProvider(app.id)).value;
    final of = previews?.of;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (of != null) ...[
          NCard(
            child: Row(
              children: [
                Icon(Icons.merge_type_rounded, color: c.muted),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        s.appPreviewOf(of.parent, of.number),
                        style: const TextStyle(fontWeight: FontWeight.w600),
                      ),
                      if (of.title.isNotEmpty) Text(of.title),
                      const SizedBox(height: 4),
                      Text(
                        s.appPreviewOfHelp(of.parent),
                        style: TextStyle(color: c.muted, fontSize: 12.5),
                      ),
                    ],
                  ),
                ),
                if (of.url.isNotEmpty)
                  TextButton.icon(
                    onPressed: () => launchUrl(Uri.parse(of.url)),
                    icon: const Icon(Icons.open_in_new_rounded, size: 16),
                    label: Text(s.appOpenPullRequest),
                  ),
              ],
            ),
          ),
          const SizedBox(height: NectarTokens.gap),
        ],
        NCard(
          title: s.appSource,
          trailing: Wrap(
            spacing: 8,
            children: [
              if (canEdit)
                OutlinedButton(
                  onPressed: () =>
                      showSourceDialog(context, appId: app.id, source: source),
                  child: Text(s.backupEdit),
                ),
              if (canBuild)
                FilledButton.icon(
                  onPressed: busy ? null : () => _build(context, ref),
                  icon: const Icon(Icons.construction_rounded, size: 18),
                  label: Text(s.appBuildDeploy),
                ),
            ],
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              SelectableText(
                source.repoUrl,
                style: monoStyle(context, size: 13.5),
              ),
              const SizedBox(height: 2),
              Text(
                [
                  'branch ${source.branch}',
                  if (source.contextDir.isNotEmpty)
                    'folder ${source.contextDir}',
                  source.dockerfile,
                  source.hasToken ? 'private (token saved)' : 'public',
                  source.autoDeploy
                      ? 'pushes build and deploy'
                      : 'build by hand',
                  if (source.previews) s.appPreviewsSwitch.toLowerCase(),
                ].join(' · '),
                style: TextStyle(color: c.muted, fontSize: 12.5),
              ),
              if (canEdit && source.webhookSecret.isNotEmpty) ...[
                const SizedBox(height: 12),
                _WebhookBox(appId: app.id, source: source),
              ],
            ],
          ),
        ),
        if (source.previews && of == null) ...[
          const SizedBox(height: NectarTokens.gap),
          _PreviewsCard(
            app: app,
            previews: previews?.previews ?? const [],
            canDelete: canEdit,
          ),
        ],
        const SizedBox(height: NectarTokens.gap),
        NCard(
          title: s.appBuilds,
          child: AsyncBlock(
            value: builds,
            onRetry: () => ref.invalidate(buildsProvider(app.id)),
            data: (list) => list.isEmpty
                ? Text(s.appNoBuildYet, style: TextStyle(color: c.muted))
                : Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      for (final b in list)
                        _BuildRow(app: app, info: b, canCancel: canBuild),
                    ],
                  ),
          ),
        ),
      ],
    );
  }
}

class _WebhookBox extends ConsumerWidget {
  const _WebhookBox({required this.appId, required this.source});

  final int appId;
  final AppSourceInfo source;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    Widget row(String label, String value) => Row(
      children: [
        SizedBox(
          width: 70,
          child: Text(label, style: TextStyle(color: c.muted, fontSize: 12.5)),
        ),
        Expanded(
          child: SelectableText(
            value,
            maxLines: 1,
            style: monoStyle(context, size: 12.5),
          ),
        ),
        IconButton(
          tooltip: s.backupCopy,
          icon: const Icon(Icons.copy_rounded, size: 16),
          onPressed: () {
            copyText(context, value, done: s.backupCopied);
          },
        ),
      ],
    );
    return Container(
      padding: const EdgeInsets.fromLTRB(12, 8, 4, 8),
      decoration: BoxDecoration(
        color: c.elevated,
        borderRadius: BorderRadius.circular(NectarTokens.radiusSmall),
        border: Border.all(color: c.border),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            s.appPushWebhook,
            style: const TextStyle(fontWeight: FontWeight.w600),
          ),
          Text(
            s.appGithubSettingsWebhooksAddWebhook,
            style: TextStyle(color: c.muted, fontSize: 12),
          ),
          const SizedBox(height: 6),
          row('URL', source.webhookUrl),
          row('Secret', source.webhookSecret),
          Align(
            alignment: Alignment.centerLeft,
            child: TextButton(
              onPressed: () async {
                if (!await confirmAction(
                  context,
                  s.appNewWebhookSecret,
                  s.appTheOldSecretStopsWorking,
                  s.appRotate,
                )) {
                  return;
                }
                try {
                  await ref
                      .read(apiProvider)
                      .post('/apps/$appId/source/webhook-secret');
                  ref.invalidate(appSourceProvider(appId));
                } on Object catch (e) {
                  if (context.mounted) {
                    showSnack(context, describeError(e), error: true);
                  }
                }
              },
              child: Text(s.appNewSecret),
            ),
          ),
        ],
      ),
    );
  }
}

class _BuildRow extends ConsumerWidget {
  const _BuildRow({
    required this.app,
    required this.info,
    required this.canCancel,
  });

  final App app;
  final BuildInfo info;
  final bool canCancel;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final c = NectarColors.of(context);
    final b = info;
    final took = b.finishedAt != null && b.startedAt != null
        ? b.finishedAt!.difference(b.startedAt!)
        : null;
    return ListTile(
      contentPadding: EdgeInsets.zero,
      leading: StatusDot(status: buildTone(b.status)),
      title: Text(
        '#${b.number}  ${b.commitMessage.isEmpty ? b.shortSha : b.commitMessage}',
        maxLines: 1,
        overflow: TextOverflow.ellipsis,
      ),
      subtitle: Text(
        [
          '${b.branch}@${b.shortSha}',
          switch (b.trigger) {
            'webhook' => 'push',
            'api' => 'API',
            _ => 'by hand',
          },
          timeAgo(b.startedAt),
          if (took != null)
            'took ${took.inMinutes > 0 ? '${took.inMinutes}m ' : ''}${took.inSeconds % 60}s',
          if (b.running) 'building…',
          if (b.release != null) 'release #${b.release}',
          if (b.status == 'failed' && b.message.isNotEmpty) b.message,
        ].join(' · '),
        maxLines: 2,
        overflow: TextOverflow.ellipsis,
        style: TextStyle(
          fontSize: 12.5,
          color: b.status == 'failed' ? c.failed : c.muted,
        ),
      ),
      trailing: const Icon(Icons.article_outlined, size: 18),
      onTap: () => showDialog<void>(
        context: context,
        builder: (_) =>
            _BuildLogDialog(app: app, number: b.number, canCancel: canCancel),
      ),
    );
  }
}

class _BuildLogDialog extends ConsumerStatefulWidget {
  const _BuildLogDialog({
    required this.app,
    required this.number,
    required this.canCancel,
  });

  final App app;
  final int number;
  final bool canCancel;

  @override
  ConsumerState<_BuildLogDialog> createState() => _BuildLogDialogState();
}

class _BuildLogDialogState extends ConsumerState<_BuildLogDialog> {
  BuildInfo? _build;
  String _log = '';
  String _status =
      ''; // why a running build has no output yet (pod not scheduled, pulling Kaniko, …)
  Object? _error;
  Timer? _timer;
  final _scroll = ScrollController();

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _timer?.cancel();
    _scroll.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    try {
      final res =
          await ref
                  .read(apiProvider)
                  .get('/apps/${widget.app.id}/builds/${widget.number}')
              as Map;
      if (!mounted) return;
      final atEnd =
          !_scroll.hasClients ||
          _scroll.position.pixels >= _scroll.position.maxScrollExtent - 40;
      setState(() {
        _build = BuildInfo.fromJson(res['build']);
        _log = '${res['log'] ?? ''}';
        _status = '${res['status'] ?? ''}';
        _error = null;
      });
      if (atEnd) {
        WidgetsBinding.instance.addPostFrameCallback((_) {
          if (_scroll.hasClients) {
            _scroll.jumpTo(_scroll.position.maxScrollExtent);
          }
        });
      }
      if (_build!.running) {
        _timer = Timer(const Duration(seconds: 3), _load);
      } else {
        ref.invalidate(buildsProvider(widget.app.id));
      }
    } on Object catch (e) {
      if (mounted) setState(() => _error = e);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final b = _build;
    return AlertDialog(
      title: Text(
        'Build #${widget.number}${b == null ? '' : ' · ${statusLabel(b.status)}'}',
      ),
      content: SizedBox(
        width: 900,
        height: 520,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (b != null)
              Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: SelectableText(
                  '${b.commitSha}${b.commitMessage.isEmpty ? '' : '  ${b.commitMessage}'}\n${b.image}${b.message.isEmpty ? '' : '\n${b.message}'}',
                  style: monoStyle(
                    context,
                    size: 12,
                    color: NectarColors.of(context).muted,
                  ),
                ),
              ),
            if (_error != null) ErrorBlock(error: _error!, onRetry: _load),
            Expanded(
              child: Container(
                padding: const EdgeInsets.all(10),
                decoration: BoxDecoration(
                  color: const Color(0xFF0A0B0D),
                  borderRadius: BorderRadius.circular(NectarTokens.radiusSmall),
                ),
                child: SingleChildScrollView(
                  controller: _scroll,
                  child: SelectableText(
                    _log.isEmpty
                        ? (b?.running ?? true
                              ? (_status.isNotEmpty
                                    ? _status
                                    : 'Waiting for output…')
                              : 'No output.')
                        : _log,
                    style: monoStyle(
                      context,
                      size: 11.5,
                      color: const Color(0xFFD9DBE0),
                    ),
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
      actions: [
        if (b != null && b.running && widget.canCancel)
          TextButton(
            onPressed: () async {
              try {
                await ref
                    .read(apiProvider)
                    .post(
                      '/apps/${widget.app.id}/builds/${widget.number}/cancel',
                    );
                _load();
              } on Object catch (e) {
                if (context.mounted) {
                  showSnack(context, describeError(e), error: true);
                }
              }
            },
            child: Text(s.appCancelBuild),
          ),
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(s.actionClose),
        ),
      ],
    );
  }
}

/// Connect an image app to a Git repository, or change its source.
Future<void> showSourceDialog(
  BuildContext context, {
  required int appId,
  AppSourceInfo? source,
}) => showDialog<void>(
  context: context,
  builder: (_) => _SourceDialog(appId: appId, source: source),
);

class _SourceDialog extends ConsumerStatefulWidget {
  const _SourceDialog({required this.appId, this.source});

  final int appId;
  final AppSourceInfo? source;

  @override
  ConsumerState<_SourceDialog> createState() => _SourceDialogState();
}

class _SourceDialogState extends ConsumerState<_SourceDialog> {
  late final _repo = TextEditingController(text: widget.source?.repoUrl ?? '');
  late final _branch = TextEditingController(
    text: widget.source?.branch ?? 'main',
  );
  late final _context = TextEditingController(
    text: widget.source?.contextDir ?? '',
  );
  late final _dockerfile = TextEditingController(
    text: widget.source?.dockerfile ?? 'Dockerfile',
  );
  final _token = TextEditingController();
  late bool _auto = widget.source?.autoDeploy ?? true;
  late bool _previews = widget.source?.previews ?? false;
  late bool _previewDbs = widget.source?.previewDatabases ?? false;
  late bool _previewForks = widget.source?.previewForks ?? false;
  late int _previewTtl = widget.source?.previewTtlDays ?? 14;
  bool _removeToken = false;
  bool _busy = false;

  @override
  void dispose() {
    for (final c in [_repo, _branch, _context, _dockerfile, _token]) {
      c.dispose();
    }
    super.dispose();
  }

  Future<void> _save() async {
    setState(() => _busy = true);
    try {
      await ref
          .read(apiProvider)
          .put(
            '/apps/${widget.appId}/source',
            data: {
              'repo_url': _repo.text.trim(),
              'branch': _branch.text.trim(),
              'context_dir': _context.text.trim(),
              'dockerfile': _dockerfile.text.trim(),
              'auto_deploy': _auto,
              'previews': _previews,
              'preview_databases': _previewDbs,
              'preview_forks': _previewForks,
              'preview_ttl_days': _previewTtl,
              if (_token.text.trim().isNotEmpty)
                'token': _token.text.trim()
              else if (_removeToken)
                'token': '',
            },
          );
      ref.invalidate(appSourceProvider(widget.appId));
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    return AlertDialog(
      title: Text(widget.source == null ? 'Build from Git' : 'Git source'),
      content: SizedBox(
        width: 520,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              GitSourceFields(
                repo: _repo,
                branch: _branch,
                contextDir: _context,
                dockerfile: _dockerfile,
                token: _token,
                hasToken: widget.source?.hasToken ?? false,
              ),
              if (widget.source?.hasToken ?? false)
                CheckboxListTile(
                  contentPadding: EdgeInsets.zero,
                  controlAffinity: ListTileControlAffinity.leading,
                  value: _removeToken,
                  onChanged: (v) => setState(() => _removeToken = v ?? false),
                  title: Text(s.appRemoveTheSavedTokenPublic),
                ),
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                value: _auto,
                onChanged: (v) => setState(() => _auto = v),
                title: Text(s.appBuildAndDeployOnEvery),
                subtitle: Text(
                  s.appNeedsThePushWebhookIn,
                  style: TextStyle(color: c.muted, fontSize: 12.5),
                ),
              ),
              const Divider(height: 24),
              Text(
                s.appPreviewEnvironments,
                style: const TextStyle(fontWeight: FontWeight.w600),
              ),
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                value: _previews,
                onChanged: (v) => setState(() => _previews = v),
                title: Text(s.appPreviewsSwitch),
                subtitle: Text(
                  s.appPreviewsHelp,
                  style: TextStyle(color: c.muted, fontSize: 12.5),
                ),
              ),
              if (_previews) ...[
                SwitchListTile(
                  contentPadding: EdgeInsets.zero,
                  value: _previewDbs,
                  onChanged: (v) => setState(() => _previewDbs = v),
                  title: Text(s.appPreviewDatabases),
                  subtitle: Text(
                    s.appPreviewDatabasesHelp,
                    style: TextStyle(color: c.muted, fontSize: 12.5),
                  ),
                ),
                SwitchListTile(
                  contentPadding: EdgeInsets.zero,
                  value: _previewForks,
                  onChanged: (v) => setState(() => _previewForks = v),
                  title: Text(s.appPreviewForks),
                  subtitle: Text(
                    s.appPreviewForksHelp,
                    style: TextStyle(color: c.warning, fontSize: 12.5),
                  ),
                ),
                Row(
                  children: [
                    Expanded(child: Text(s.appPreviewTtl)),
                    DropdownButton<int>(
                      value: const [0, 3, 7, 14, 30].contains(_previewTtl)
                          ? _previewTtl
                          : 14,
                      items: [
                        for (final d in const [3, 7, 14, 30])
                          DropdownMenuItem(
                            value: d,
                            child: Text(s.appPreviewDays(d)),
                          ),
                        DropdownMenuItem(
                          value: 0,
                          child: Text(s.appPreviewTtlNever),
                        ),
                      ],
                      onChanged: (v) => setState(() => _previewTtl = v ?? 14),
                    ),
                  ],
                ),
              ],
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(s.actionCancel),
        ),
        FilledButton(
          onPressed: _busy ? null : _save,
          child: Text(_busy ? 'Checking the repository…' : 'Save'),
        ),
      ],
    );
  }
}

/// The repository fields (shared by the New App wizard and the source dialog).
class GitSourceFields extends StatelessWidget {
  const GitSourceFields({
    super.key,
    required this.repo,
    required this.branch,
    required this.contextDir,
    required this.dockerfile,
    required this.token,
    this.hasToken = false,
  });

  final TextEditingController repo;
  final TextEditingController branch;
  final TextEditingController contextDir;
  final TextEditingController dockerfile;
  final TextEditingController token;
  final bool hasToken;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        TextField(
          controller: repo,
          style: monoStyle(context, size: 13.5),
          autocorrect: false,
          decoration: InputDecoration(
            labelText: s.appRepository,
            hintText: 'https://github.com/acme/api.git',
          ),
        ),
        const SizedBox(height: 10),
        Wrap(
          spacing: 12,
          runSpacing: 10,
          children: [
            SizedBox(
              width: 160,
              child: TextField(
                controller: branch,
                decoration: InputDecoration(
                  labelText: s.appBranch,
                  isDense: true,
                ),
              ),
            ),
            SizedBox(
              width: 160,
              child: TextField(
                controller: contextDir,
                decoration: InputDecoration(
                  labelText: s.backupFolder,
                  hintText: s.appRoot,
                  isDense: true,
                ),
              ),
            ),
            SizedBox(
              width: 160,
              child: TextField(
                controller: dockerfile,
                decoration: InputDecoration(
                  labelText: s.appDockerfile,
                  isDense: true,
                ),
              ),
            ),
          ],
        ),
        const SizedBox(height: 10),
        TextField(
          controller: token,
          obscureText: true,
          decoration: InputDecoration(
            labelText: hasToken
                ? 'Access token (saved; type to replace)'
                : 'Access token (private repositories)',
            helperText: s.appGithubAFineGrainedToken,
          ),
        ),
        const SizedBox(height: 6),
        Text(
          s.appTheRepositoryNeedsADockerfile,
          style: TextStyle(color: c.muted, fontSize: 12.5),
        ),
      ],
    );
  }
}

/// The open pull requests' previews of an app.
class _PreviewsCard extends ConsumerWidget {
  const _PreviewsCard({
    required this.app,
    required this.previews,
    required this.canDelete,
  });

  final App app;
  final List<PreviewInfo> previews;
  final bool canDelete;

  Future<void> _delete(
    BuildContext context,
    WidgetRef ref,
    PreviewInfo p,
  ) async {
    final s = S.of(context);
    if (!await confirmAction(
      context,
      s.appDeletePreview(p.slug),
      s.appDeletePreviewHelp,
      s.actionDelete,
      danger: true,
    )) {
      return;
    }
    try {
      await ref.read(apiProvider).delete('/apps/${p.appId}');
      ref.invalidate(previewsProvider(app.id));
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    return NCard(
      title: s.appPreviews,
      child: previews.isEmpty
          ? Text(s.appNoPreviews, style: TextStyle(color: c.muted))
          : Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                for (final p in previews)
                  ListTile(
                    contentPadding: EdgeInsets.zero,
                    leading: StatusDot(status: p.status),
                    title: Text(
                      '#${p.number} ${p.title}',
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                    subtitle: Text(
                      [
                        p.slug,
                        if (p.author.isNotEmpty) p.author,
                        if (p.commitSha.length >= 7)
                          p.commitSha.substring(0, 7),
                      ].join(' · '),
                      style: monoStyle(context, size: 12, color: c.muted),
                    ),
                    onTap: () => context.go('/apps/${p.appId}'),
                    trailing: Wrap(
                      spacing: 4,
                      children: [
                        if (p.liveUrl.isNotEmpty)
                          IconButton(
                            tooltip: s.appOpenPreview,
                            icon: const Icon(
                              Icons.open_in_new_rounded,
                              size: 18,
                            ),
                            onPressed: () => launchUrl(Uri.parse(p.liveUrl)),
                          ),
                        if (p.url.isNotEmpty)
                          IconButton(
                            tooltip: s.appOpenPullRequest,
                            icon: const Icon(
                              Icons.merge_type_rounded,
                              size: 18,
                            ),
                            onPressed: () => launchUrl(Uri.parse(p.url)),
                          ),
                        if (canDelete)
                          IconButton(
                            tooltip: s.actionDelete,
                            icon: Icon(
                              Icons.delete_outline_rounded,
                              size: 18,
                              color: c.failed,
                            ),
                            onPressed: () => _delete(context, ref, p),
                          ),
                      ],
                    ),
                  ),
              ],
            ),
    );
  }
}

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/status.dart';

String buildTone(String s) => switch (s) {
      'succeeded' => 'running',
      'running' => 'deploying',
      'canceled' => 'stopped',
      _ => 'failed',
    };

/// Builds: the Git source (repository, branch, webhook) and the build history with logs.
class BuildsTab extends ConsumerWidget {
  const BuildsTab({super.key, required this.app, required this.source});

  final App app;
  final AppSourceInfo source;

  Future<void> _build(BuildContext context, WidgetRef ref) async {
    try {
      final res = await ref.read(apiProvider).post('/apps/${app.id}/builds', data: {'deploy': true});
      ref.invalidate(buildsProvider(app.id));
      if (context.mounted) showSnack(context, 'Build #${(res as Map)['number']} started');
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final session = ref.watch(sessionProvider);
    final canBuild = session.can('app.deploy', projectId: app.projectId);
    final canEdit = session.can('app.edit', projectId: app.projectId);
    final builds = ref.watch(buildsProvider(app.id));
    final c = NectarColors.of(context);
    final busy = builds.value?.any((b) => b.running) ?? false;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        NCard(
          title: 'Source',
          trailing: Wrap(
            spacing: 8,
            children: [
              if (canEdit)
                OutlinedButton(onPressed: () => showSourceDialog(context, appId: app.id, source: source), child: const Text('Edit')),
              if (canBuild)
                FilledButton.icon(
                  onPressed: busy ? null : () => _build(context, ref),
                  icon: const Icon(Icons.construction_rounded, size: 18),
                  label: const Text('Build & deploy'),
                ),
            ],
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              SelectableText(source.repoUrl, style: monoStyle(context, size: 13.5)),
              const SizedBox(height: 2),
              Text(
                [
                  'branch ${source.branch}',
                  if (source.contextDir.isNotEmpty) 'folder ${source.contextDir}',
                  source.dockerfile,
                  source.hasToken ? 'private (token saved)' : 'public',
                  source.autoDeploy ? 'pushes build and deploy' : 'build by hand',
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
        const SizedBox(height: NectarTokens.gap),
        NCard(
          title: 'Builds',
          child: AsyncBlock(
            value: builds,
            onRetry: () => ref.invalidate(buildsProvider(app.id)),
            data: (list) => list.isEmpty
                ? Text('No build yet.', style: TextStyle(color: c.muted))
                : Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [for (final b in list) _BuildRow(app: app, info: b, canCancel: canBuild)],
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
    final c = NectarColors.of(context);
    Widget row(String label, String value) => Row(
          children: [
            SizedBox(width: 70, child: Text(label, style: TextStyle(color: c.muted, fontSize: 12.5))),
            Expanded(child: SelectableText(value, maxLines: 1, style: monoStyle(context, size: 12.5))),
            IconButton(
              tooltip: 'Copy',
              icon: const Icon(Icons.copy_rounded, size: 16),
              onPressed: () {
                Clipboard.setData(ClipboardData(text: value));
                showSnack(context, 'Copied');
              },
            ),
          ],
        );
    return Container(
      padding: const EdgeInsets.fromLTRB(12, 8, 4, 8),
      decoration: BoxDecoration(color: c.elevated, borderRadius: BorderRadius.circular(NectarTokens.radiusSmall), border: Border.all(color: c.border)),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text('Push webhook', style: const TextStyle(fontWeight: FontWeight.w600)),
          Text(
            'GitHub: Settings → Webhooks → Add webhook, content type application/json, this secret. GitLab: Webhooks, '
            'secret token = this secret, push events. Gitea/Forgejo: the same as GitHub.',
            style: TextStyle(color: c.muted, fontSize: 12),
          ),
          const SizedBox(height: 6),
          row('URL', source.webhookUrl),
          row('Secret', source.webhookSecret),
          Align(
            alignment: Alignment.centerLeft,
            child: TextButton(
              onPressed: () async {
                if (!await confirmAction(context, 'New webhook secret?', 'The old secret stops working: update it in your Git host.', 'Rotate')) return;
                try {
                  await ref.read(apiProvider).post('/apps/$appId/source/webhook-secret');
                  ref.invalidate(appSourceProvider(appId));
                } on Object catch (e) {
                  if (context.mounted) showSnack(context, describeError(e), error: true);
                }
              },
              child: const Text('New secret'),
            ),
          ),
        ],
      ),
    );
  }
}

class _BuildRow extends ConsumerWidget {
  const _BuildRow({required this.app, required this.info, required this.canCancel});

  final App app;
  final BuildInfo info;
  final bool canCancel;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final c = NectarColors.of(context);
    final b = info;
    final took = b.finishedAt != null && b.startedAt != null ? b.finishedAt!.difference(b.startedAt!) : null;
    return ListTile(
      contentPadding: EdgeInsets.zero,
      leading: StatusDot(status: buildTone(b.status)),
      title: Text('#${b.number}  ${b.commitMessage.isEmpty ? b.shortSha : b.commitMessage}', maxLines: 1, overflow: TextOverflow.ellipsis),
      subtitle: Text(
        [
          '${b.branch}@${b.shortSha}',
          switch (b.trigger) { 'webhook' => 'push', 'api' => 'API', _ => 'by hand' },
          timeAgo(b.startedAt),
          if (took != null) 'took ${took.inMinutes > 0 ? '${took.inMinutes}m ' : ''}${took.inSeconds % 60}s',
          if (b.running) 'building…',
          if (b.release != null) 'release #${b.release}',
          if (b.status == 'failed' && b.message.isNotEmpty) b.message,
        ].join(' · '),
        maxLines: 2,
        overflow: TextOverflow.ellipsis,
        style: TextStyle(fontSize: 12.5, color: b.status == 'failed' ? c.failed : c.muted),
      ),
      trailing: const Icon(Icons.article_outlined, size: 18),
      onTap: () => showDialog<void>(context: context, builder: (_) => _BuildLogDialog(app: app, number: b.number, canCancel: canCancel)),
    );
  }
}

class _BuildLogDialog extends ConsumerStatefulWidget {
  const _BuildLogDialog({required this.app, required this.number, required this.canCancel});

  final App app;
  final int number;
  final bool canCancel;

  @override
  ConsumerState<_BuildLogDialog> createState() => _BuildLogDialogState();
}

class _BuildLogDialogState extends ConsumerState<_BuildLogDialog> {
  BuildInfo? _build;
  String _log = '';
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
      final res = await ref.read(apiProvider).get('/apps/${widget.app.id}/builds/${widget.number}') as Map;
      if (!mounted) return;
      final atEnd = !_scroll.hasClients || _scroll.position.pixels >= _scroll.position.maxScrollExtent - 40;
      setState(() {
        _build = BuildInfo.fromJson(res['build']);
        _log = '${res['log'] ?? ''}';
        _error = null;
      });
      if (atEnd) {
        WidgetsBinding.instance.addPostFrameCallback((_) {
          if (_scroll.hasClients) _scroll.jumpTo(_scroll.position.maxScrollExtent);
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
    final b = _build;
    return AlertDialog(
      title: Text('Build #${widget.number}${b == null ? '' : ' · ${statusLabel(b.status)}'}'),
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
                  style: monoStyle(context, size: 12, color: NectarColors.of(context).muted),
                ),
              ),
            if (_error != null) ErrorBlock(error: _error!, onRetry: _load),
            Expanded(
              child: Container(
                padding: const EdgeInsets.all(10),
                decoration: BoxDecoration(color: const Color(0xFF0A0B0D), borderRadius: BorderRadius.circular(NectarTokens.radiusSmall)),
                child: SingleChildScrollView(
                  controller: _scroll,
                  child: SelectableText(
                    _log.isEmpty ? (b?.running ?? true ? 'Waiting for output…' : 'No output.') : _log,
                    style: monoStyle(context, size: 11.5, color: const Color(0xFFD9DBE0)),
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
                await ref.read(apiProvider).post('/apps/${widget.app.id}/builds/${widget.number}/cancel');
                _load();
              } on Object catch (e) {
                if (context.mounted) showSnack(context, describeError(e), error: true);
              }
            },
            child: const Text('Cancel build'),
          ),
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('Close')),
      ],
    );
  }
}

/// Connect an image app to a Git repository, or change its source.
Future<void> showSourceDialog(BuildContext context, {required int appId, AppSourceInfo? source}) =>
    showDialog<void>(context: context, builder: (_) => _SourceDialog(appId: appId, source: source));

class _SourceDialog extends ConsumerStatefulWidget {
  const _SourceDialog({required this.appId, this.source});

  final int appId;
  final AppSourceInfo? source;

  @override
  ConsumerState<_SourceDialog> createState() => _SourceDialogState();
}

class _SourceDialogState extends ConsumerState<_SourceDialog> {
  late final _repo = TextEditingController(text: widget.source?.repoUrl ?? '');
  late final _branch = TextEditingController(text: widget.source?.branch ?? 'main');
  late final _context = TextEditingController(text: widget.source?.contextDir ?? '');
  late final _dockerfile = TextEditingController(text: widget.source?.dockerfile ?? 'Dockerfile');
  final _token = TextEditingController();
  late bool _auto = widget.source?.autoDeploy ?? true;
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
      await ref.read(apiProvider).put('/apps/${widget.appId}/source', data: {
        'repo_url': _repo.text.trim(),
        'branch': _branch.text.trim(),
        'context_dir': _context.text.trim(),
        'dockerfile': _dockerfile.text.trim(),
        'auto_deploy': _auto,
        if (_token.text.trim().isNotEmpty) 'token': _token.text.trim() else if (_removeToken) 'token': '',
      });
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
              GitSourceFields(repo: _repo, branch: _branch, contextDir: _context, dockerfile: _dockerfile, token: _token, hasToken: widget.source?.hasToken ?? false),
              if (widget.source?.hasToken ?? false)
                CheckboxListTile(
                  contentPadding: EdgeInsets.zero,
                  controlAffinity: ListTileControlAffinity.leading,
                  value: _removeToken,
                  onChanged: (v) => setState(() => _removeToken = v ?? false),
                  title: const Text('Remove the saved token (public repository)'),
                ),
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                value: _auto,
                onChanged: (v) => setState(() => _auto = v),
                title: const Text('Build and deploy on every push'),
                subtitle: Text('Needs the push webhook in your Git host', style: TextStyle(color: c.muted, fontSize: 12.5)),
              ),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
        FilledButton(onPressed: _busy ? null : _save, child: Text(_busy ? 'Checking the repository…' : 'Save')),
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
    final c = NectarColors.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        TextField(
          controller: repo,
          style: monoStyle(context, size: 13.5),
          autocorrect: false,
          decoration: const InputDecoration(labelText: 'Repository', hintText: 'https://github.com/acme/api.git'),
        ),
        const SizedBox(height: 10),
        Wrap(
          spacing: 12,
          runSpacing: 10,
          children: [
            SizedBox(width: 160, child: TextField(controller: branch, decoration: const InputDecoration(labelText: 'Branch', isDense: true))),
            SizedBox(width: 160, child: TextField(controller: contextDir, decoration: const InputDecoration(labelText: 'Folder', hintText: '(root)', isDense: true))),
            SizedBox(width: 160, child: TextField(controller: dockerfile, decoration: const InputDecoration(labelText: 'Dockerfile', isDense: true))),
          ],
        ),
        const SizedBox(height: 10),
        TextField(
          controller: token,
          obscureText: true,
          decoration: InputDecoration(
            labelText: hasToken ? 'Access token (saved; type to replace)' : 'Access token (private repositories)',
            helperText: 'GitHub: a fine-grained token with Contents: read. GitLab: read_repository.',
          ),
        ),
        const SizedBox(height: 6),
        Text('The repository needs a Dockerfile. Images are built in the cluster with Kaniko and kept in the built-in registry.',
            style: TextStyle(color: c.muted, fontSize: 12.5)),
      ],
    );
  }
}

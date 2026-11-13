import 'dart:async';

import 'package:material_ui/material_ui.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:nectar_apps_ui/theme/theme.dart';

import '../../api/client.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../generated/l10n.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/status.dart';

/// A Helm chart installed into a project (GET /projects/{id}/helm).
class HelmRelease {
  const HelmRelease({
    required this.name,
    required this.repo,
    required this.chart,
    required this.version,
    required this.status,
    required this.message,
    required this.revision,
    required this.helmStatus,
    required this.chartVersion,
    required this.appVersion,
    required this.repoUsername,
    required this.hasPassword,
  });

  final String name;
  final String repo;
  final String chart;
  final String version; // requested ("" = latest)
  final String
  status; // installing | upgrading | rolling_back | uninstalling | deployed | failed
  final String message;
  final int revision;
  final String helmStatus;
  final String chartVersion;
  final String appVersion;
  final String repoUsername;
  final bool hasPassword;

  bool get busy => const [
    'installing',
    'upgrading',
    'rolling_back',
    'uninstalling',
  ].contains(status);

  factory HelmRelease.fromJson(Map m) => HelmRelease(
    name: '${m['name'] ?? ''}',
    repo: '${m['repo'] ?? ''}',
    chart: '${m['chart'] ?? ''}',
    version: '${m['version'] ?? ''}',
    status: '${m['status'] ?? ''}',
    message: '${m['message'] ?? ''}',
    revision: (m['revision'] as num?)?.toInt() ?? 0,
    helmStatus: '${m['helm_status'] ?? ''}',
    chartVersion: '${m['chart_version'] ?? ''}',
    appVersion: '${m['app_version'] ?? ''}',
    repoUsername: '${m['repo_username'] ?? ''}',
    hasPassword: m['has_password'] == true,
  );
}

final helmReleasesProvider = FutureProvider.autoDispose
    .family<List<HelmRelease>, int>((ref, projectId) async {
      final res = await ref.watch(apiProvider).get('/projects/$projectId/helm');
      return [
        for (final m in (res as List? ?? const []))
          HelmRelease.fromJson(m as Map),
      ];
    });

String helmStatusLabel(S s, String status) => switch (status) {
  'installing' => s.helmInstalling,
  'upgrading' => s.helmUpgrading,
  'rolling_back' => s.helmRollingBack,
  'uninstalling' => s.helmUninstalling,
  'deployed' => s.helmDeployed,
  'failed' => s.helmFailed,
  _ => status,
};

String _tone(String status) => switch (status) {
  'deployed' => 'running',
  'failed' => 'failed',
  'installing' ||
  'upgrading' ||
  'rolling_back' ||
  'uninstalling' => 'deploying',
  _ => status,
};

/// The project's Helm charts (project page).
class HelmSection extends ConsumerStatefulWidget {
  const HelmSection({super.key, required this.projectId});

  final int projectId;

  @override
  ConsumerState<HelmSection> createState() => _HelmSectionState();
}

class _HelmSectionState extends ConsumerState<HelmSection> {
  Timer? _poll;

  @override
  void dispose() {
    _poll?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final id = widget.projectId;
    final canManage = ref.watch(
      sessionProvider.select((x) => x.can('helm.manage', projectId: id)),
    );
    final releases = ref.watch(helmReleasesProvider(id));
    // refresh while an operation runs
    final busy = releases.value?.any((r) => r.busy) ?? false;
    if (busy && _poll == null) {
      _poll = Timer.periodic(
        const Duration(seconds: 3),
        (_) => ref.invalidate(helmReleasesProvider(id)),
      );
    } else if (!busy && _poll != null) {
      _poll!.cancel();
      _poll = null;
    }
    final list = releases.value ?? const <HelmRelease>[];
    if (!canManage && list.isEmpty) return const SizedBox.shrink();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SizedBox(height: NectarTokens.gapLarge),
        SectionLabel(
          s.helmTitle,
          trailing: canManage
              ? TextButton.icon(
                  onPressed: () => showHelmInstallDialog(context, id),
                  icon: const Icon(Icons.add_rounded, size: 18),
                  label: Text(s.helmInstall),
                )
              : null,
        ),
        AsyncBlock(
          value: releases,
          loadingHeight: 60,
          onRetry: () => ref.invalidate(helmReleasesProvider(id)),
          data: (list) {
            if (list.isEmpty) {
              return NCard(
                child: Text(
                  s.helmNone,
                  style: TextStyle(color: c.muted, fontSize: 13),
                ),
              );
            }
            return NCard(
              padding: const EdgeInsets.symmetric(vertical: 6),
              child: Column(
                children: [
                  for (var i = 0; i < list.length; i++) ...[
                    if (i > 0) const Divider(height: 1),
                    ListTile(
                      leading: const Icon(
                        Icons.sailing_rounded,
                        color: NectarTokens.amber,
                      ),
                      title: Text(
                        list[i].name,
                        style: const TextStyle(fontWeight: FontWeight.w600),
                      ),
                      subtitle: Text(
                        [
                          list[i].chartVersion.isEmpty
                              ? list[i].chart
                              : '${list[i].chart} ${list[i].chartVersion}',
                          if (list[i].appVersion.isNotEmpty)
                            'app ${list[i].appVersion}',
                          if (list[i].revision > 0)
                            s.helmRevision(list[i].revision),
                          if (list[i].status == 'failed' &&
                              list[i].message.isNotEmpty)
                            list[i].message.split('\n').last,
                        ].join(' · '),
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                        style: TextStyle(color: c.muted, fontSize: 12.5),
                      ),
                      trailing: StatusPill(
                        status: _tone(list[i].status),
                        label: helmStatusLabel(s, list[i].status),
                      ),
                      onTap: () =>
                          showHelmReleaseDialog(context, id, list[i].name),
                    ),
                  ],
                ],
              ),
            );
          },
        ),
      ],
    );
  }
}

// ------------------------------------------------------------------ install --

Future<void> showHelmInstallDialog(BuildContext context, int projectId) =>
    showDialog<void>(
      context: context,
      builder: (_) => _HelmEditDialog(projectId: projectId),
    );

/// Install a chart, or (with [release]) change its chart reference, version and values.
class _HelmEditDialog extends ConsumerStatefulWidget {
  const _HelmEditDialog({
    required this.projectId,
    this.release,
    this.values = '',
  });

  final int projectId;
  final HelmRelease? release;
  final String values;

  @override
  ConsumerState<_HelmEditDialog> createState() => _HelmEditDialogState();
}

class _HelmEditDialogState extends ConsumerState<_HelmEditDialog> {
  late final _name = TextEditingController(text: widget.release?.name ?? '');
  late final _repo = TextEditingController(text: widget.release?.repo ?? '');
  late final _chart = TextEditingController(text: widget.release?.chart ?? '');
  late final _version = TextEditingController(
    text: widget.release?.version ?? '',
  );
  late final _values = TextEditingController(text: widget.values);
  late final _user = TextEditingController(
    text: widget.release?.repoUsername ?? '',
  );
  final _pass = TextEditingController();
  late bool _login = widget.release?.repoUsername.isNotEmpty ?? false;
  bool _busy = false;

  @override
  void dispose() {
    for (final c in [_name, _repo, _chart, _version, _values, _user, _pass]) {
      c.dispose();
    }
    super.dispose();
  }

  Future<void> _save() async {
    final s = S.of(context);
    final r = widget.release;
    setState(() => _busy = true);
    final body = {
      if (r == null) 'name': _name.text.trim(),
      'repo': _repo.text.trim(),
      'chart': _chart.text.trim(),
      'version': _version.text.trim(),
      'values': _values.text,
      'repo_username': _login ? _user.text.trim() : '',
      if (_login && _pass.text.isNotEmpty) 'repo_password': _pass.text,
    };
    try {
      final api = ref.read(apiProvider);
      final base = '/projects/${widget.projectId}/helm';
      if (r == null) {
        await api.post(base, data: body);
      } else {
        await api.put('$base/${r.name}', data: body);
      }
      ref.invalidate(helmReleasesProvider(widget.projectId));
      if (!mounted) return;
      Navigator.pop(context, true);
      showSnack(
        context,
        r == null ? s.helmInstallStarted : s.helmUpgradeStarted,
      );
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
    final r = widget.release;
    return AlertDialog(
      title: Text(r == null ? s.helmInstall : s.helmUpgradeTitle(r.name)),
      content: SizedBox(
        width: 620,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (r == null) ...[
                Text(
                  s.helmInstallHelp,
                  style: TextStyle(color: c.muted, fontSize: 12.5),
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: _name,
                  decoration: InputDecoration(
                    labelText: s.helmName,
                    hintText: 'grafana',
                  ),
                ),
                const SizedBox(height: 10),
              ],
              TextField(
                controller: _repo,
                decoration: InputDecoration(
                  labelText: s.helmRepo,
                  hintText: 'https://grafana.github.io/helm-charts',
                  helperText: s.helmRepoHelp,
                ),
              ),
              const SizedBox(height: 10),
              Row(
                children: [
                  Expanded(
                    child: TextField(
                      controller: _chart,
                      decoration: InputDecoration(
                        labelText: s.helmChart,
                        hintText: 'grafana',
                      ),
                    ),
                  ),
                  const SizedBox(width: 12),
                  SizedBox(
                    width: 170,
                    child: TextField(
                      controller: _version,
                      decoration: InputDecoration(
                        labelText: s.helmVersion,
                        hintText: s.helmVersionLatest,
                      ),
                    ),
                  ),
                ],
              ),
              CheckboxListTile(
                contentPadding: EdgeInsets.zero,
                controlAffinity: ListTileControlAffinity.leading,
                value: _login,
                onChanged: (v) => setState(() => _login = v ?? false),
                title: Text(s.helmRepoLogin),
              ),
              if (_login)
                Row(
                  children: [
                    Expanded(
                      child: TextField(
                        controller: _user,
                        decoration: InputDecoration(labelText: s.helmRepoUser),
                      ),
                    ),
                    const SizedBox(width: 12),
                    Expanded(
                      child: TextField(
                        controller: _pass,
                        obscureText: true,
                        decoration: InputDecoration(
                          labelText: s.helmRepoPassword,
                          hintText: r?.hasPassword ?? false
                              ? s.helmPasswordKept
                              : null,
                        ),
                      ),
                    ),
                  ],
                ),
              const SizedBox(height: 12),
              TextField(
                controller: _values,
                minLines: 8,
                maxLines: 18,
                style: monoStyle(context, size: 12.5),
                decoration: InputDecoration(
                  labelText: s.helmValues,
                  hintText: 'replicaCount: 1\nservice:\n  port: 80',
                  alignLabelWithHint: true,
                  helperText: s.helmValuesHelp,
                  helperMaxLines: 3,
                ),
              ),
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
          child: Text(r == null ? s.helmInstall : s.helmUpgrade),
        ),
      ],
    );
  }
}

// ------------------------------------------------------------------- detail --

Future<void> showHelmReleaseDialog(
  BuildContext context,
  int projectId,
  String name,
) => showDialog<void>(
  context: context,
  builder: (_) => _HelmReleaseDialog(projectId: projectId, name: name),
);

class _HelmReleaseDialog extends ConsumerStatefulWidget {
  const _HelmReleaseDialog({required this.projectId, required this.name});

  final int projectId;
  final String name;

  @override
  ConsumerState<_HelmReleaseDialog> createState() => _HelmReleaseDialogState();
}

class _HelmReleaseDialogState extends ConsumerState<_HelmReleaseDialog> {
  Map<String, dynamic>? _d;
  Object? _error;
  Timer? _poll;

  String get _base => '/projects/${widget.projectId}/helm/${widget.name}';

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _poll?.cancel();
    super.dispose();
  }

  Future<void> _load() async {
    try {
      final d = await ref.read(apiProvider).get(_base) as Map;
      if (!mounted) return;
      setState(() {
        _d = Map<String, dynamic>.from(d);
        _error = null;
      });
      final busy = HelmRelease.fromJson(d['release'] as Map).busy;
      if (busy && _poll == null) {
        _poll = Timer.periodic(const Duration(seconds: 3), (_) => _load());
      } else if (!busy) {
        _poll?.cancel();
        _poll = null;
      }
    } on Object catch (e) {
      if (mounted) setState(() => _error = e);
    }
  }

  void _changed() {
    ref.invalidate(helmReleasesProvider(widget.projectId));
    _load();
  }

  Future<void> _upgrade(HelmRelease r) async {
    final done = await showDialog<bool>(
      context: context,
      builder: (_) => _HelmEditDialog(
        projectId: widget.projectId,
        release: r,
        values: '${_d?['values'] ?? ''}',
      ),
    );
    if (done == true) _changed();
  }

  Future<void> _rollback(int revision) async {
    final s = S.of(context);
    if (!await confirmAction(
      context,
      s.helmRollbackTitle(revision),
      s.helmRollbackBody,
      s.helmRollback,
    )) {
      return;
    }
    try {
      await ref
          .read(apiProvider)
          .post('$_base/rollback', data: {'revision': revision});
      _changed();
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    }
  }

  Future<void> _uninstall() async {
    final s = S.of(context);
    if (!await confirmAction(
      context,
      s.helmUninstallTitle(widget.name),
      s.helmUninstallBody,
      s.helmUninstall,
      danger: true,
    )) {
      return;
    }
    try {
      await ref.read(apiProvider).delete(_base);
      ref.invalidate(helmReleasesProvider(widget.projectId));
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final canManage = ref.watch(
      sessionProvider.select(
        (x) => x.can('helm.manage', projectId: widget.projectId),
      ),
    );
    final d = _d;
    Widget body;
    if (d == null) {
      body = _error == null
          ? const Padding(
              padding: EdgeInsets.all(24),
              child: Center(child: CircularProgressIndicator()),
            )
          : Text(describeError(_error!), style: TextStyle(color: c.failed));
    } else {
      final r = HelmRelease.fromJson(d['release'] as Map);
      final history = [
        for (final h in (d['history'] as List? ?? const [])) h as Map,
      ];
      final workloads = [
        for (final w in (d['workloads'] as List? ?? const [])) w as Map,
      ];
      final services = [
        for (final x in (d['services'] as List? ?? const [])) '$x',
      ];
      final output = '${d['output'] ?? ''}'.trim();
      final notes = history.isEmpty
          ? ''
          : '${history.first['notes'] ?? ''}'.trim();
      final mono = monoStyle(context, size: 12);
      body = Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              StatusPill(
                status: _tone(r.status),
                label: helmStatusLabel(s, r.status),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                  '${r.repo} · ${r.chart}${r.chartVersion.isEmpty ? '' : ' ${r.chartVersion}'}',
                  style: TextStyle(color: c.muted, fontSize: 12.5),
                  overflow: TextOverflow.ellipsis,
                ),
              ),
            ],
          ),
          if (r.status == 'failed' && r.message.isNotEmpty) ...[
            const SizedBox(height: 10),
            SelectableText(r.message, style: mono.copyWith(color: c.failed)),
          ],
          if (workloads.isNotEmpty) ...[
            const SizedBox(height: 14),
            Text(
              s.helmWorkloads,
              style: const TextStyle(fontWeight: FontWeight.w600),
            ),
            for (final w in workloads)
              Padding(
                padding: const EdgeInsets.only(top: 4),
                child: Text(
                  '${w['kind']} ${w['name']} · ${w['ready'] ?? 0}/${w['replicas'] ?? 0}',
                  style: mono,
                ),
              ),
          ],
          if (services.isNotEmpty) ...[
            const SizedBox(height: 14),
            Text(
              s.helmServices,
              style: const TextStyle(fontWeight: FontWeight.w600),
            ),
            Text(
              s.helmServicesHelp,
              style: TextStyle(color: c.muted, fontSize: 12),
            ),
            for (final x in services)
              Row(
                children: [
                  Expanded(child: SelectableText(x, style: mono)),
                  IconButton(
                    tooltip: s.helmCopy,
                    icon: const Icon(Icons.copy_rounded, size: 16),
                    onPressed: () {
                      Clipboard.setData(ClipboardData(text: x));
                      showSnack(context, s.helmCopied);
                    },
                  ),
                ],
              ),
          ],
          if (notes.isNotEmpty) ...[
            const SizedBox(height: 14),
            Text(
              s.helmNotes,
              style: const TextStyle(fontWeight: FontWeight.w600),
            ),
            const SizedBox(height: 4),
            Container(
              constraints: const BoxConstraints(maxHeight: 180),
              child: SingleChildScrollView(
                child: SelectableText(notes, style: mono),
              ),
            ),
          ],
          if (history.isNotEmpty) ...[
            const SizedBox(height: 14),
            Text(
              s.helmHistory,
              style: const TextStyle(fontWeight: FontWeight.w600),
            ),
            for (final h in history)
              ListTile(
                dense: true,
                contentPadding: EdgeInsets.zero,
                title: Text(
                  '#${h['revision']} · ${h['chart']} ${h['chart_version']} · ${h['status']}',
                  style: mono,
                ),
                subtitle: Text(
                  [
                    if (h['updated'] != null)
                      timeAgo(DateTime.tryParse('${h['updated']}')),
                    '${h['description'] ?? ''}',
                  ].where((x) => x.isNotEmpty).join(' · '),
                  style: TextStyle(color: c.muted, fontSize: 12),
                ),
                trailing: canManage && h['status'] != 'deployed' && !r.busy
                    ? TextButton(
                        onPressed: () =>
                            _rollback((h['revision'] as num).toInt()),
                        child: Text(s.helmRollback),
                      )
                    : null,
              ),
          ],
          if (output.isNotEmpty) ...[
            const SizedBox(height: 14),
            ExpansionTile(
              tilePadding: EdgeInsets.zero,
              title: Text(
                s.helmOutput,
                style: const TextStyle(fontWeight: FontWeight.w600),
              ),
              children: [
                Container(
                  constraints: const BoxConstraints(maxHeight: 240),
                  child: SingleChildScrollView(
                    child: SelectableText(output, style: mono),
                  ),
                ),
              ],
            ),
          ],
        ],
      );
    }
    final r = d == null ? null : HelmRelease.fromJson(d['release'] as Map);
    return AlertDialog(
      title: Text(widget.name),
      content: SizedBox(width: 640, child: SingleChildScrollView(child: body)),
      actions: [
        if (canManage && r != null && !r.busy) ...[
          TextButton(
            onPressed: _uninstall,
            style: TextButton.styleFrom(
              foregroundColor: NectarColors.of(context).failed,
            ),
            child: Text(s.helmUninstall),
          ),
          TextButton(onPressed: () => _upgrade(r), child: Text(s.helmUpgrade)),
        ],
        FilledButton(
          onPressed: () => Navigator.pop(context),
          child: Text(s.actionClose),
        ),
      ],
    );
  }
}

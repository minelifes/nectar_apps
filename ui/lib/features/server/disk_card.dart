import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/gauge.dart';

/// Disk usage of the nodes and the panel's data volume, cleanup actions and retention settings.
class DiskCard extends ConsumerStatefulWidget {
  const DiskCard({super.key});

  @override
  ConsumerState<DiskCard> createState() => _DiskCardState();
}

class _DiskCardState extends ConsumerState<DiskCard> {
  bool _cleaning = false;
  bool _pruning = false;

  Future<void> _cleanup() async {
    setState(() => _cleaning = true);
    try {
      final res = await ref.read(apiProvider).post('/server/cleanup');
      final r = res is Map ? Map<String, dynamic>.from(res['result'] as Map? ?? const {}) : const <String, dynamic>{};
      int n(String k) => (r[k] as num?)?.toInt() ?? 0;
      final parts = [
        if (n('files') > 0) '${n('files')} old uploads (${formatBytes(n('bytes_freed'))})',
        if (n('uploads') > 0) '${n('uploads')} unfinished uploads',
        if (n('audit_entries') > 0) '${n('audit_entries')} audit entries',
        if (n('resolved_alerts') > 0) '${n('resolved_alerts')} old alerts',
      ];
      ref.invalidate(serverDiskProvider);
      if (mounted) showSnack(context, parts.isEmpty ? 'Nothing to clean up' : 'Removed ${parts.join(', ')}');
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _cleaning = false);
    }
  }

  Future<void> _pruneImages() async {
    final s = S.of(context);
    final ok = await confirmAction(
      context,
      s.serverRemoveUnusedImages,
      s.serverDeletesTheContainerImagesThat,
      s.serverRemoveImages,
    );
    if (!ok || !mounted) return;
    setState(() => _pruning = true);
    try {
      await ref.read(apiProvider).post('/server/image-cleanup', data: <String, dynamic>{});
      ref.invalidate(serverDiskProvider);
      if (mounted) showSnack(context, s.serverImageCleanupStarted);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _pruning = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final disk = ref.watch(serverDiskProvider);
    final session = ref.watch(sessionProvider);
    final canManage = session.can('server.manage');
    final canSettings = session.can('settings.manage');
    final c = NectarColors.of(context);
    return NCard(
      title: s.serverDisk,
      trailing: IconButton(
        tooltip: s.actionRefresh,
        icon: const Icon(Icons.refresh_rounded, size: 18),
        onPressed: () => ref.invalidate(serverDiskProvider),
      ),
      child: AsyncBlock(
        value: disk,
        loadingHeight: 120,
        onRetry: () => ref.invalidate(serverDiskProvider),
        data: (d) => Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            for (final n in d.nodes) ...[
              if (d.nodes.length > 1) Padding(padding: const EdgeInsets.only(bottom: 6), child: Text(n.name, style: Theme.of(context).textTheme.titleSmall)),
              if (n.error.isNotEmpty)
                Text(n.error, style: TextStyle(color: c.muted, fontSize: 12.5))
              else ...[
                UsageBar(
                  label: s.serverServerDisk,
                  value: n.fsTotal > 0 ? n.fsUsed / n.fsTotal : 0,
                  trailing: '${formatBytes(n.fsUsed)} of ${formatBytes(n.fsTotal)}',
                ),
                if (n.imagesTotal > 0) ...[
                  const SizedBox(height: 10),
                  UsageBar(
                    label: n.imagesTotal == n.fsTotal ? 'Container images (part of the disk)' : 'Container images',
                    value: n.imagesUsed / n.imagesTotal,
                    trailing: formatBytes(n.imagesUsed),
                  ),
                ],
              ],
              const SizedBox(height: 16),
            ],
            if (d.volumeTotal > 0) ...[
              UsageBar(
                label: s.serverPanelDataVolume,
                value: (d.volumeTotal - d.volumeFree) / d.volumeTotal,
                trailing: '${formatBytes(d.volumeTotal - d.volumeFree)} of ${formatBytes(d.volumeTotal)}',
              ),
              const SizedBox(height: 8),
            ],
            Wrap(
              spacing: 24,
              runSpacing: 6,
              children: [
                _Fact('Database', formatBytes(d.databaseBytes)),
                _Fact('Uploaded files', '${formatBytes(d.artifactBytes)} · ${d.artifactFiles} ${d.artifactFiles == 1 ? 'file' : 'files'}'),
                if (d.uploadBytes > 0) _Fact('Unfinished uploads', formatBytes(d.uploadBytes)),
              ],
            ),
            const SizedBox(height: 16),
            Text(
              s.serverKeptTheLastUploadsPer(d.settings.keepArtifacts, d.settings.auditDays, d.settings.alertDays),
              style: TextStyle(color: c.muted, fontSize: 12.5),
            ),
            if (canManage || canSettings) ...[
              const SizedBox(height: 12),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  if (canManage)
                    OutlinedButton.icon(
                      onPressed: _cleaning ? null : _cleanup,
                      icon: const Icon(Icons.cleaning_services_outlined, size: 18),
                      label: Text(_cleaning ? 'Cleaning…' : 'Clean up now'),
                    ),
                  if (canManage)
                    OutlinedButton.icon(
                      onPressed: _pruning || d.cleanupRunning || d.nodes.isEmpty ? null : _pruneImages,
                      icon: const Icon(Icons.layers_clear_outlined, size: 18),
                      label: Text(d.cleanupRunning ? 'Removing images…' : 'Remove unused images'),
                    ),
                  if (canSettings)
                    TextButton.icon(
                      onPressed: () => showDialog<void>(context: context, builder: (_) => _RetentionDialog(settings: d.settings)),
                      icon: const Icon(Icons.tune_rounded, size: 18),
                      label: Text(s.serverRetention),
                    ),
                ],
              ),
            ],
            if (d.cleanups.isNotEmpty) ...[
              const SizedBox(height: 16),
              Text(s.serverImageCleanups, style: Theme.of(context).textTheme.titleSmall),
              const SizedBox(height: 6),
              for (final j in d.cleanups.take(5)) _CleanupRow(job: j),
            ],
          ],
        ),
      ),
    );
  }
}

class _Fact extends StatelessWidget {
  const _Fact(this.label, this.value);

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    return Text.rich(
      TextSpan(children: [
        TextSpan(text: '$label  ', style: TextStyle(color: c.muted)),
        TextSpan(text: value, style: const TextStyle(fontWeight: FontWeight.w600)),
      ]),
      style: const TextStyle(fontSize: 13),
    );
  }
}

class _CleanupRow extends StatelessWidget {
  const _CleanupRow({required this.job});

  final ImageCleanup job;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final (icon, color, text) = switch (job.status) {
      'succeeded' => (Icons.check_circle_rounded, c.running, job.removed == 1 ? '1 image removed' : '${job.removed} images removed'),
      'failed' => (Icons.error_rounded, c.failed, job.message.isEmpty ? 'failed' : 'failed: ${job.message}'),
      _ => (Icons.autorenew_rounded, c.deploying, 'running…'),
    };
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: Row(
        children: [
          Icon(icon, size: 16, color: color),
          const SizedBox(width: 8),
          Expanded(child: Text('${job.node} · $text', style: const TextStyle(fontSize: 13))),
          Text(timeAgo(job.started), style: TextStyle(color: c.muted, fontSize: 12)),
        ],
      ),
    );
  }
}

class _RetentionDialog extends ConsumerStatefulWidget {
  const _RetentionDialog({required this.settings});

  final HousekeepingSettings settings;

  @override
  ConsumerState<_RetentionDialog> createState() => _RetentionDialogState();
}

class _RetentionDialogState extends ConsumerState<_RetentionDialog> {
  late final _keep = TextEditingController(text: '${widget.settings.keepArtifacts}');
  late final _audit = TextEditingController(text: '${widget.settings.auditDays}');
  late final _alerts = TextEditingController(text: '${widget.settings.alertDays}');
  late final _dataDir = TextEditingController(text: widget.settings.k3sDataDir);
  late final _images = TextEditingController(text: widget.settings.prepullImages.join('\n'));
  late bool _prepull = widget.settings.prepull;
  late bool _logHistory = widget.settings.logHistory;
  late final _logDays = TextEditingController(text: '${widget.settings.logDays}');
  late final _logMax = TextEditingController(text: '${widget.settings.logMaxMb}');
  bool _saving = false;
  String? _error;

  @override
  void dispose() {
    for (final c in [_keep, _audit, _alerts, _dataDir, _images, _logDays, _logMax]) {
      c.dispose();
    }
    super.dispose();
  }

  Future<void> _save() async {
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      await ref.read(apiProvider).put('/settings/housekeeping', data: HousekeepingSettings(
        keepArtifacts: int.tryParse(_keep.text.trim()) ?? 0,
        auditDays: int.tryParse(_audit.text.trim()) ?? 0,
        alertDays: int.tryParse(_alerts.text.trim()) ?? 0,
        k3sDataDir: _dataDir.text.trim(),
        prepull: _prepull,
        logHistory: _logHistory,
        logDays: int.tryParse(_logDays.text.trim()) ?? 0,
        logMaxMb: int.tryParse(_logMax.text.trim()) ?? 0,
        prepullImages: [
          for (final l in _images.text.split(RegExp(r'[\s,]+')))
            if (l.trim().isNotEmpty) l.trim(),
        ],
      ).toJson());
      ref.invalidate(serverDiskProvider);
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    Widget num(TextEditingController ctl, String label, String help) => Padding(
          padding: const EdgeInsets.only(bottom: 12),
          child: TextField(controller: ctl, keyboardType: TextInputType.number, decoration: InputDecoration(labelText: label, helperText: help)),
        );
    return AlertDialog(
      title: Text(s.serverRetention),
      content: SizedBox(
        width: 420,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              num(_keep, 'Uploads kept per app', '1–100; uploads used by a live release are always kept'),
              num(_audit, 'Audit log, days', '30–3650'),
              num(_alerts, 'Resolved alerts, days', '7–3650'),
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                value: _logHistory,
                onChanged: (v) => setState(() => _logHistory = v),
                title: Text(s.serverKeepAppLogs),
                subtitle: Text(s.serverStoresEveryAppSOutput),
              ),
              if (_logHistory) ...[
                num(_logDays, 'Log history, days', '1–90'),
                num(_logMax, 'Log history per app, MB', '16–20480; the oldest hours go first'),
              ],
              TextField(
                controller: _dataDir,
                decoration: InputDecoration(labelText: s.serverK3sDataDirectory, helperText: s.serverUsedToFindCrictlFor),
              ),
              const SizedBox(height: 8),
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                value: _prepull,
                onChanged: (v) => setState(() => _prepull = v),
                title: Text(s.serverKeepImagesOnEveryNode),
                subtitle: Text(s.serverPrePullsBusyboxAndThe),
              ),
              if (_prepull)
                TextField(
                  controller: _images,
                  minLines: 1,
                  maxLines: 4,
                  decoration: InputDecoration(labelText: s.serverExtraImages, helperText: s.serverOnePerLineEG),
                ),
              if (_error != null) Padding(padding: const EdgeInsets.only(top: 12), child: Text(_error!, style: TextStyle(color: c.failed))),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: Text(s.actionCancel)),
        FilledButton(onPressed: _saving ? null : _save, child: Text(_saving ? 'Saving…' : 'Save')),
      ],
    );
  }
}

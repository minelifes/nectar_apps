import 'dart:convert';

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
import 'plan_dialog.dart';
import 'target_dialog.dart';

/// Backup plans, targets and the run history.
class BackupsPage extends ConsumerStatefulWidget {
  const BackupsPage({super.key});

  @override
  ConsumerState<BackupsPage> createState() => _BackupsPageState();
}

class _BackupsPageState extends ConsumerState<BackupsPage> {
  String _tab = 'Plans';

  Future<void> _refresh() async {
    ref.invalidate(backupPlansProvider);
    ref.invalidate(backupTargetsProvider);
    ref.invalidate(backupRunsProvider);
  }

  @override
  Widget build(BuildContext context) {
    final session = ref.watch(sessionProvider);
    if (!session.can('backup.manage')) {
      return const PageScaffold(
        title: 'Backups',
        children: [
          NCard(child: EmptyState(icon: Icons.lock_outline_rounded, title: 'No access', message: 'Your role does not include backup.manage.')),
        ],
      );
    }
    final tabs = <(String, Widget Function())>[
      ('Plans', () => _Plans(canRestore: session.can('backup.restore'))),
      ('Targets', () => const _Targets()),
      ('History', () => const _History()),
    ];
    final i = tabs.indexWhere((t) => t.$1 == _tab).clamp(0, tabs.length - 1);
    return PageScaffold(
      title: 'Backups',
      subtitle: 'Volumes, database dumps and the panel itself, encrypted with restic to S3, Backblaze B2, SFTP or a folder.',
      onRefresh: _refresh,
      children: [
        NTabs(index: i, labels: [for (final t in tabs) t.$1], onChanged: (n) => setState(() => _tab = tabs[n].$1)),
        const SizedBox(height: NectarTokens.gap),
        tabs[i].$2(),
      ],
    );
  }
}

String scheduleLabel(String s) => switch (s) {
      '' => 'manual only',
      '@hourly' || '0 * * * *' => 'every hour',
      '0 */6 * * *' => 'every 6 hours',
      '@daily' || '0 3 * * *' => 'daily at 03:00 UTC',
      '@weekly' || '0 3 * * 0' => 'weekly, Sunday 03:00 UTC',
      _ => s,
    };

IconData scopeIcon(String scope) => switch (scope) {
      'app' => Icons.apps_rounded,
      'addon' => Icons.storage_rounded,
      _ => Icons.hexagon_outlined,
    };

// -------------------------------------------------------------------- plans --

class _Plans extends ConsumerWidget {
  const _Plans({required this.canRestore});

  final bool canRestore;

  Future<void> _run(BuildContext context, WidgetRef ref, BackupPlan p) async {
    try {
      await ref.read(apiProvider).post('/backup/plans/${p.id}/run');
      ref.invalidate(backupRunsProvider);
      ref.invalidate(backupPlansProvider);
      if (context.mounted) showSnack(context, 'Backup of ${p.subject} started');
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  Future<void> _delete(BuildContext context, WidgetRef ref, BackupPlan p) async {
    if (!await confirmAction(context, 'Delete this plan?', 'No more backups of ${p.subject} to ${p.targetName}. Existing snapshots stay in the repository.', 'Delete',
        danger: true)) {
      return;
    }
    try {
      await ref.read(apiProvider).delete('/backup/plans/${p.id}');
      ref.invalidate(backupPlansProvider);
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final plans = ref.watch(backupPlansProvider);
    final targets = ref.watch(backupTargetsProvider);
    ref.watch(backupRunsProvider); // keeps plans fresh while a run is in progress
    final c = NectarColors.of(context);
    final noTargets = targets.hasValue && targets.value!.isEmpty;
    return NCard(
      title: 'What is backed up',
      trailing: FilledButton.icon(
        onPressed: noTargets || !targets.hasValue ? null : () => showPlanDialog(context, ref, targets: targets.value!),
        icon: const Icon(Icons.add_rounded, size: 18),
        label: const Text('Add plan'),
      ),
      child: AsyncBlock(
        value: plans,
        onRetry: () => ref.invalidate(backupPlansProvider),
        data: (list) {
          if (noTargets) {
            return const EmptyState(
              icon: Icons.cloud_upload_outlined,
              title: 'Add a target first',
              message: 'A target is where backups go: an S3 bucket (AWS, Backblaze B2, R2, MinIO), an SFTP server or a folder on this server.',
            );
          }
          if (list.isEmpty) {
            return const EmptyState(
              icon: Icons.backup_outlined,
              title: 'Nothing is backed up yet',
              message: 'Add a plan for the panel itself (needed to rebuild a lost server), for databases and for apps with volumes.',
            );
          }
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              for (var i = 0; i < list.length; i++) ...[
                if (i > 0) const Divider(height: 1),
                _PlanRow(
                  plan: list[i],
                  canRestore: canRestore,
                  onRun: () => _run(context, ref, list[i]),
                  onEdit: targets.hasValue ? () => showPlanDialog(context, ref, targets: targets.value!, plan: list[i]) : null,
                  onDelete: () => _delete(context, ref, list[i]),
                ),
              ],
              const SizedBox(height: 8),
              Text(
                'Backups of volumes read files while the app runs; databases are dumped consistently. Times are UTC.',
                style: TextStyle(color: c.muted, fontSize: 12),
              ),
            ],
          );
        },
      ),
    );
  }
}

class _PlanRow extends StatelessWidget {
  const _PlanRow({required this.plan, required this.canRestore, required this.onRun, required this.onEdit, required this.onDelete});

  final BackupPlan plan;
  final bool canRestore;
  final VoidCallback onRun;
  final VoidCallback? onEdit;
  final VoidCallback onDelete;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final last = plan.lastRun;
    final keep = [
      if (plan.keepDaily > 0) '${plan.keepDaily} daily',
      if (plan.keepWeekly > 0) '${plan.keepWeekly} weekly',
      if (plan.keepMonthly > 0) '${plan.keepMonthly} monthly',
    ].join(', ');
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 10),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(scopeIcon(plan.scope), color: plan.enabled ? NectarTokens.amber : c.muted),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(plan.subject, style: const TextStyle(fontWeight: FontWeight.w600)),
                Text('to ${plan.targetName} · ${plan.enabled ? scheduleLabel(plan.schedule) : 'paused'} · keeps $keep',
                    style: TextStyle(color: c.muted, fontSize: 12.5)),
                const SizedBox(height: 4),
                if (last != null)
                  Row(
                    children: [
                      StatusDot(status: last.status == 'succeeded' ? 'running' : 'failed'),
                      const SizedBox(width: 6),
                      Expanded(
                        child: Text(
                          '${last.status == 'succeeded' ? 'Last backup' : 'Last backup failed'} ${timeAgo(last.finishedAt ?? last.startedAt)}'
                          '${last.message.isEmpty ? '' : ' · ${last.message.split('\n').first}'}',
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                          style: TextStyle(fontSize: 12.5, color: last.status == 'failed' ? c.failed : c.text),
                        ),
                      ),
                    ],
                  )
                else
                  Text('Not backed up yet', style: TextStyle(fontSize: 12.5, color: c.warning)),
                if (plan.nextRun != null) Text('Next: ${_when(plan.nextRun!)}', style: TextStyle(color: c.muted, fontSize: 12)),
              ],
            ),
          ),
          Wrap(
            children: [
              IconButton(tooltip: 'Back up now', icon: const Icon(Icons.play_arrow_rounded), onPressed: onRun),
              if (plan.scope != 'panel')
                IconButton(
                  tooltip: canRestore ? 'Snapshots and restore' : 'Snapshots',
                  icon: const Icon(Icons.history_rounded),
                  onPressed: () => showDialog<void>(context: context, builder: (_) => _SnapshotsDialog(plan: plan, canRestore: canRestore)),
                ),
              IconButton(tooltip: 'Edit', icon: const Icon(Icons.edit_outlined, size: 20), onPressed: onEdit),
              IconButton(tooltip: 'Delete', icon: const Icon(Icons.delete_outline_rounded, size: 20), onPressed: onDelete),
            ],
          ),
        ],
      ),
    );
  }

  static String _when(DateTime t) {
    final l = t.toLocal();
    String two(int n) => n.toString().padLeft(2, '0');
    return '${l.year}-${two(l.month)}-${two(l.day)} ${two(l.hour)}:${two(l.minute)}';
  }
}

class _SnapshotsDialog extends ConsumerStatefulWidget {
  const _SnapshotsDialog({required this.plan, required this.canRestore});

  final BackupPlan plan;
  final bool canRestore;

  @override
  ConsumerState<_SnapshotsDialog> createState() => _SnapshotsDialogState();
}

class _SnapshotsDialogState extends ConsumerState<_SnapshotsDialog> {
  late Future<List<BackupSnapshot>> _snaps = _load();

  Future<List<BackupSnapshot>> _load() async =>
      listOf(await ref.read(apiProvider).get('/backup/plans/${widget.plan.id}/snapshots'), BackupSnapshot.fromJson);

  Future<void> _restore(BackupSnapshot s) async {
    final what = widget.plan.scope == 'app'
        ? 'The app is stopped, its volumes are replaced with the snapshot, and it starts again.'
        : 'The database is replaced with the dump in the snapshot.';
    final ok = await confirmAction(context, 'Restore ${s.shortId}?', '$what Changes since then are lost.', 'Restore', danger: true);
    if (!ok || !mounted) return;
    try {
      await ref.read(apiProvider).post('/backup/plans/${widget.plan.id}/restore', data: {'snapshot': s.id});
      ref.invalidate(backupRunsProvider);
      if (mounted) {
        Navigator.pop(context);
        showSnack(context, 'Restore started: follow it under History');
      }
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    return AlertDialog(
      title: Text('Snapshots · ${widget.plan.subject}'),
      content: SizedBox(
        width: 560,
        height: 380,
        child: FutureBuilder<List<BackupSnapshot>>(
          future: _snaps,
          builder: (context, snap) {
            if (snap.hasError) return ErrorBlock(error: snap.error!, onRetry: () => setState(() => _snaps = _load()));
            if (!snap.hasData) {
              return Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  const CircularProgressIndicator(),
                  const SizedBox(height: 12),
                  Text('Reading the repository…', style: TextStyle(color: c.muted)),
                ],
              );
            }
            final list = snap.data!;
            if (list.isEmpty) return const Center(child: Text('No snapshots yet.'));
            return ListView(
              children: [
                for (final s in list)
                  ListTile(
                    leading: const Icon(Icons.inventory_2_outlined),
                    title: Text(s.time == null ? s.shortId : '${timeAgo(s.time)} · ${s.shortId}'),
                    subtitle: Text(s.paths.join(', '), style: monoStyle(context, size: 11.5, color: c.muted)),
                    trailing: widget.canRestore ? TextButton(onPressed: () => _restore(s), child: const Text('Restore')) : null,
                  ),
              ],
            );
          },
        ),
      ),
      actions: [TextButton(onPressed: () => Navigator.pop(context), child: const Text('Close'))],
    );
  }
}

// ------------------------------------------------------------------ targets --

class _Targets extends ConsumerWidget {
  const _Targets();

  Future<void> _check(BuildContext context, WidgetRef ref, BackupTarget t) async {
    showSnack(context, 'Checking ${t.name}…');
    try {
      final res = await ref.read(apiProvider).post('/backup/targets/${t.id}/check');
      if (context.mounted) showSnack(context, '${t.name} works · ${(res as Map)['snapshots']} snapshots');
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  Future<void> _kit(BuildContext context, WidgetRef ref, BackupTarget t) async {
    try {
      final kit = await ref.read(apiProvider).get('/backup/targets/${t.id}/recovery-kit');
      if (!context.mounted) return;
      final text = const JsonEncoder.withIndent('  ').convert(kit);
      await showDialog<void>(
        context: context,
        builder: (ctx) => AlertDialog(
          title: const Text('Recovery kit'),
          content: SizedBox(
            width: 620,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                const Text(
                  'Save this as nectar-recovery-kit.json somewhere safe, together with your master key. '
                  'On a new server: install.sh --restore-kit nectar-recovery-kit.json --master-key <key>',
                ),
                const SizedBox(height: 12),
                Container(
                  constraints: const BoxConstraints(maxHeight: 300),
                  padding: const EdgeInsets.all(10),
                  decoration: BoxDecoration(color: const Color(0xFF0A0B0D), borderRadius: BorderRadius.circular(NectarTokens.radiusSmall)),
                  child: SingleChildScrollView(child: SelectableText(text, style: monoStyle(ctx, size: 12, color: const Color(0xFFD9DBE0)))),
                ),
                const SizedBox(height: 8),
                Text('It holds the repository password and access keys: anyone with it and the master key can read your backups.',
                    style: TextStyle(color: NectarColors.of(ctx).warning, fontSize: 12.5)),
              ],
            ),
          ),
          actions: [
            TextButton(
              onPressed: () {
                Clipboard.setData(ClipboardData(text: text));
                showSnack(ctx, 'Copied');
              },
              child: const Text('Copy'),
            ),
            FilledButton(onPressed: () => Navigator.pop(ctx), child: const Text('Done')),
          ],
        ),
      );
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  Future<void> _delete(BuildContext context, WidgetRef ref, BackupTarget t) async {
    if (!await confirmAction(context, 'Remove ${t.name}?', 'The panel forgets the target. The backups stay where they are.', 'Remove', danger: true)) return;
    try {
      await ref.read(apiProvider).delete('/backup/targets/${t.id}');
      ref.invalidate(backupTargetsProvider);
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final targets = ref.watch(backupTargetsProvider);
    final c = NectarColors.of(context);
    return NCard(
      title: 'Where backups go',
      trailing: FilledButton.icon(
        onPressed: () => showTargetDialog(context, ref),
        icon: const Icon(Icons.add_rounded, size: 18),
        label: const Text('Add target'),
      ),
      child: AsyncBlock(
        value: targets,
        onRetry: () => ref.invalidate(backupTargetsProvider),
        data: (list) => list.isEmpty
            ? Text('No target yet.', style: TextStyle(color: c.muted))
            : Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  for (final t in list)
                    ListTile(
                      contentPadding: EdgeInsets.zero,
                      leading: Icon(t.kind == 'local' ? Icons.folder_outlined : (t.kind == 'sftp' ? Icons.dns_outlined : Icons.cloud_outlined)),
                      title: Text(t.name, style: const TextStyle(fontWeight: FontWeight.w600)),
                      subtitle: Text('${t.repository} · ${t.plans} ${t.plans == 1 ? 'plan' : 'plans'}', style: monoStyle(context, size: 12, color: c.muted)),
                      trailing: Wrap(
                        children: [
                          IconButton(tooltip: 'Check', icon: const Icon(Icons.fact_check_outlined, size: 20), onPressed: () => _check(context, ref, t)),
                          IconButton(tooltip: 'Recovery kit', icon: const Icon(Icons.key_rounded, size: 20), onPressed: () => _kit(context, ref, t)),
                          IconButton(tooltip: 'Edit', icon: const Icon(Icons.edit_outlined, size: 20), onPressed: () => showTargetDialog(context, ref, target: t)),
                          IconButton(tooltip: 'Remove', icon: const Icon(Icons.delete_outline_rounded, size: 20), onPressed: () => _delete(context, ref, t)),
                        ],
                      ),
                    ),
                ],
              ),
      ),
    );
  }
}

// ------------------------------------------------------------------ history --

class _History extends ConsumerWidget {
  const _History();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final runs = ref.watch(backupRunsProvider);
    final c = NectarColors.of(context);
    return NCard(
      child: AsyncBlock(
        value: runs,
        onRetry: () => ref.invalidate(backupRunsProvider),
        data: (list) => list.isEmpty
            ? Text('No backup has run yet.', style: TextStyle(color: c.muted))
            : Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  for (final r in list)
                    ListTile(
                      contentPadding: EdgeInsets.zero,
                      leading: StatusDot(status: switch (r.status) { 'succeeded' => 'running', 'failed' => 'failed', _ => 'deploying' }),
                      title: Text('${r.kind == 'restore' ? 'Restore' : 'Backup'} · ${r.subject}'),
                      subtitle: Text(
                        [
                          timeAgo(r.startedAt),
                          if (r.finishedAt != null && r.startedAt != null) 'took ${_took(r.finishedAt!.difference(r.startedAt!))}',
                          if (r.status == 'running') 'running…',
                          if (r.message.isNotEmpty) r.message,
                        ].join(' · '),
                        maxLines: 4,
                        overflow: TextOverflow.ellipsis,
                        style: TextStyle(color: r.status == 'failed' ? c.failed : c.muted, fontSize: 12.5),
                      ),
                    ),
                ],
              ),
      ),
    );
  }

  static String _took(Duration d) => d.inMinutes >= 1 ? '${d.inMinutes}m ${d.inSeconds % 60}s' : '${d.inSeconds}s';
}

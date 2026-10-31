import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/status.dart';
import 'update_dialog.dart';
import 'update_settings_card.dart';

const componentNames = {'panel': 'Nectar Apps panel', 'gateway': 'nectar-gateway', 'k3s': 'k3s'};

/// Settings → Updates: versions of the panel, the gateway and k3s, the running update, policies and history.
class UpdatesPage extends ConsumerWidget {
  const UpdatesPage({super.key});

  Future<void> _check(BuildContext context, WidgetRef ref) async {
    final s = S.of(context);
    try {
      await ref.read(apiProvider).post('/updates/check');
      ref.invalidate(updatesProvider);
      if (context.mounted) showSnack(context, s.updateCheckedForUpdates);
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final session = ref.watch(sessionProvider);
    if (!session.can('server.view')) {
      return PageScaffold(
        title: s.updateUpdates,
        children: [NCard(child: EmptyState(icon: Icons.lock_outline_rounded, title: s.noAccess, message: s.gatewayYourRoleDoesNotInclude))],
      );
    }
    final manage = session.can('server.manage');
    final data = ref.watch(updatesProvider);
    return PageScaffold(
      title: s.updateUpdates,
      subtitle: s.updateThePanelNectarGatewayAnd,
      onRefresh: () async => ref.invalidate(updatesProvider),
      actions: [
        if (manage)
          OutlinedButton.icon(onPressed: () => _check(context, ref), icon: const Icon(Icons.refresh_rounded, size: 18), label: Text(s.updateCheckNow)),
      ],
      children: [
        AsyncBlock(
          value: data,
          onRetry: () => ref.invalidate(updatesProvider),
          data: (o) => Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (o.active != null) ...[_ActiveRun(run: o.active!), const SizedBox(height: NectarTokens.gap)],
              _Versions(overview: o, manage: manage),
              const SizedBox(height: NectarTokens.gap),
              if (manage) ...[UpdateSettingsCard(settings: o.settings), const SizedBox(height: NectarTokens.gap)],
              _History(overview: o, manage: manage),
            ],
          ),
        ),
      ],
    );
  }
}

// ----------------------------------------------------------------- versions --

class _Versions extends ConsumerWidget {
  const _Versions({required this.overview, required this.manage});

  final UpdatesOverview overview;
  final bool manage;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final o = overview;
    return NCard(
      title: s.updateVersions,
      trailing: Text(
        o.checkedAt == null ? 'not checked yet' : 'checked ${timeAgo(o.checkedAt)}',
        style: TextStyle(color: c.muted, fontSize: 12.5),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          for (final (i, comp) in const ['panel', 'gateway', 'k3s'].indexed) ...[
            if (i > 0) const Divider(height: 1),
            _ComponentRow(
              component: comp,
              state: o.component(comp),
              policy: '${(o.settings[comp] as Map?)?['policy'] ?? ''}',
              busy: o.active != null,
              manage: manage,
            ),
          ],
          if (o.bad.isNotEmpty) ...[
            const SizedBox(height: 10),
            for (final e in o.bad.entries)
              Row(
                children: [
                  Icon(Icons.block_rounded, size: 16, color: c.failed),
                  const SizedBox(width: 6),
                  Expanded(
                    child: Text('${e.key.replaceFirst('@', ' ')} is not installed automatically: ${e.value}',
                        style: TextStyle(fontSize: 12.5, color: c.muted), maxLines: 2, overflow: TextOverflow.ellipsis),
                  ),
                  if (manage)
                    TextButton(
                      onPressed: () async {
                        try {
                          await ref.read(apiProvider).delete('/updates/bad/${Uri.encodeComponent(e.key)}');
                          ref.invalidate(updatesProvider);
                        } on Object catch (err) {
                          if (context.mounted) showSnack(context, describeError(err), error: true);
                        }
                      },
                      child: Text(s.updateAllowAgain),
                    ),
                ],
              ),
          ],
        ],
      ),
    );
  }
}

class _ComponentRow extends ConsumerWidget {
  const _ComponentRow({required this.component, required this.state, required this.policy, required this.busy, required this.manage});

  final String component;
  final ComponentUpdate state;
  final String policy;
  final bool busy;
  final bool manage;

  String _statusText() {
    if (policy == 'off') return 'Checks are off';
    if (state.error.isNotEmpty && state.installed.isEmpty) return state.error;
    if (state.hasUpdate) {
      final r = state.available.first;
      return '${r.version} available${r.security ? ' · security fix' : ''}${r.breaking ? ' · breaking' : ''}';
    }
    if (state.latest == null) return state.error.isNotEmpty ? state.error : 'Not checked yet';
    return 'Up to date';
  }

  Future<void> _k3sUpdate(BuildContext context, WidgetRef ref) async {
    final s = S.of(context);
    final v = state.available.first.version;
    if (!await confirmAction(
        context,
        s.updateUpdateK3sTo(v),
        s.updateTheSystemUpgradeControllerUpgrades,
        s.updateUpgrade)) {
      return;
    }
    try {
      await ref.read(apiProvider).post('/updates/k3s', data: {'version': v});
      ref.invalidate(updatesProvider);
      if (context.mounted) showSnack(context, s.updateK3sUpgradeToStartedNode(v));
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final tone = state.hasUpdate ? 'warning' : (state.error.isNotEmpty ? 'failed' : (state.latest != null ? 'running' : 'stopped'));
    final canUpdate = manage && state.hasUpdate && !busy && (component != 'k3s' || state.controller);
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 12),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(padding: const EdgeInsets.only(top: 4), child: StatusDot(status: tone)),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Text(componentNames[component]!, style: const TextStyle(fontWeight: FontWeight.w600)),
                    const SizedBox(width: 8),
                    Text(state.installed.isEmpty ? '—' : state.installed, style: monoStyle(context, size: 12.5, color: c.muted)),
                  ],
                ),
                const SizedBox(height: 2),
                Text(_statusText(), style: TextStyle(fontSize: 13, color: state.hasUpdate ? c.text : c.muted)),
                if (state.waiting.isNotEmpty && policy != 'off')
                  Text(state.waiting, style: TextStyle(fontSize: 12.5, color: c.muted)),
                if (state.error.isNotEmpty && state.installed.isNotEmpty)
                  Text(state.error, style: TextStyle(fontSize: 12.5, color: c.warning), maxLines: 2, overflow: TextOverflow.ellipsis),
                if (component == 'k3s' && state.nodes.length > 1)
                  Padding(
                    padding: const EdgeInsets.only(top: 4),
                    child: Wrap(
                      spacing: 10,
                      children: [
                        for (final n in state.nodes)
                          Text('${n.name} ${n.version}${n.unschedulable ? ' (cordoned)' : ''}', style: monoStyle(context, size: 11.5, color: c.muted)),
                      ],
                    ),
                  ),
                if (component == 'k3s' && !state.controller && policy != 'off')
                  Text(s.updateUpdatingK3sFromHereNeeds,
                      style: TextStyle(fontSize: 12.5, color: c.muted)),
              ],
            ),
          ),
          if (canUpdate)
            FilledButton(
              onPressed: () => component == 'k3s' ? _k3sUpdate(context, ref) : showUpdateDialog(context, ref, component: component, state: state),
              child: Text(s.updateUpdate2),
            ),
        ],
      ),
    );
  }
}

// --------------------------------------------------------------- active run --

String runStateLabel(String s) => switch (s) {
      'pending' => 'Starting',
      'running' => 'Running',
      'succeeded' => 'Done',
      'failed' => 'Not started',
      'rolled_back' => 'Rolled back',
      'rollback_failed' => 'Needs attention',
      _ => s,
    };

String runTone(String s) => switch (s) {
      'succeeded' => 'running',
      'pending' || 'running' => 'deploying',
      'failed' || 'rolled_back' => 'warning',
      _ => 'failed',
    };

class _ActiveRun extends ConsumerStatefulWidget {
  const _ActiveRun({required this.run});

  final UpdateRunInfo run;

  @override
  ConsumerState<_ActiveRun> createState() => _ActiveRunState();
}

class _ActiveRunState extends ConsumerState<_ActiveRun> {
  late final UpdateInProgress _flag = ref.read(updateInProgressProvider.notifier);

  @override
  void initState() {
    super.initState();
    // keep polling through the panel's own restart
    Future.microtask(() => _flag.set(true));
  }


  @override
  Widget build(BuildContext context) {
    final l = S.of(context);
    final c = NectarColors.of(context);
    final run = widget.run;
    final panelStep = run.steps.any((s) => s.component == 'panel' && s.state == 'running');
    return NCard(
      title: l.updateUpdating(run.summary),
      trailing: StatusPill(status: 'deploying', label: runStateLabel(run.state)),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          for (final s in run.steps) RunStepRow(step: s),
          if (panelStep) ...[
            const SizedBox(height: 6),
            Text(l.updateThePanelRestartsNowThis, style: TextStyle(color: c.muted, fontSize: 12.5)),
          ],
          if (run.log.isNotEmpty) ...[
            const SizedBox(height: 10),
            RunLog(log: run.log, max: 8),
          ],
        ],
      ),
    );
  }
}

class RunStepRow extends StatelessWidget {
  const RunStepRow({super.key, required this.step});

  final UpdateStep step;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final (icon, color) = switch (step.state) {
      'done' => (Icons.check_circle_rounded, c.running),
      'skipped' => (Icons.remove_circle_outline_rounded, c.muted),
      'running' => (Icons.autorenew_rounded, NectarTokens.amber),
      'failed' => (Icons.error_rounded, c.failed),
      'rolled_back' => (Icons.undo_rounded, c.warning),
      _ => (Icons.radio_button_unchecked_rounded, c.muted),
    };
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(icon, size: 18, color: color),
          const SizedBox(width: 8),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('${componentNames[step.component] ?? step.component}  ${step.from} → ${step.to}${step.migrations ? '  · database changes' : ''}'),
                if (step.message.isNotEmpty) Text(step.message, style: TextStyle(color: c.muted, fontSize: 12.5)),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class RunLog extends StatelessWidget {
  const RunLog({super.key, required this.log, this.max = 100});

  final List<(DateTime?, String)> log;
  final int max;

  @override
  Widget build(BuildContext context) {
    final lines = log.length > max ? log.sublist(log.length - max) : log;
    String two(int n) => n.toString().padLeft(2, '0');
    return Container(
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(color: const Color(0xFF0A0B0D), borderRadius: BorderRadius.circular(NectarTokens.radiusSmall)),
      child: SelectableText(
        [
          for (final (t, m) in lines)
            '${t == null ? '' : '${two(t.hour)}:${two(t.minute)}:${two(t.second)}  '}$m',
        ].join('\n'),
        style: monoStyle(context, size: 11.5, color: const Color(0xFFD9DBE0)),
      ),
    );
  }
}

// ------------------------------------------------------------------ history --

class _History extends ConsumerWidget {
  const _History({required this.overview, required this.manage});

  final UpdatesOverview overview;
  final bool manage;

  Future<void> _rollback(BuildContext context, WidgetRef ref, UpdateRunInfo run) async {
    final l = S.of(context);
    final db = run.steps.where((s) => s.migrations && s.state == 'done').map((s) => componentNames[s.component]).join(' and ');
    final ok = await confirmAction(
      context,
      l.updateRollBack(run.summary),
      'Every component goes back to the version it had before this update.'
          '${db.isEmpty ? '' : ' The $db database is restored to its state before the update: changes made since then are lost.'}',
      l.updateRollBack2,
      danger: db.isNotEmpty,
    );
    if (!ok || !context.mounted) return;
    try {
      await ref.read(apiProvider).post('/updates/rollback', data: {'run_id': run.id});
      ref.read(updateInProgressProvider.notifier).set(true);
      ref.invalidate(updatesProvider);
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = S.of(context);
    final c = NectarColors.of(context);
    final runs = overview.runs.where((r) => !r.active).toList();
    final rollbackable = runs.where((r) => r.state == 'succeeded' && r.trigger != 'rollback').firstOrNull;
    return NCard(
      title: l.observeHistory,
      child: runs.isEmpty
          ? Text(l.updateNoUpdateHasRunOn, style: TextStyle(color: c.muted))
          : Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                for (final r in runs)
                  ExpansionTile(
                    tilePadding: EdgeInsets.zero,
                    leading: StatusDot(status: runTone(r.state)),
                    title: Text(r.summary.isEmpty ? 'Update #${r.id}' : r.summary),
                    subtitle: Text(
                      [
                        runStateLabel(r.state),
                        switch (r.trigger) { 'auto' => 'automatic', 'rollback' => 'rollback', _ => 'by hand' },
                        timeAgo(r.startedAt),
                        if (r.error.isNotEmpty) r.error,
                      ].join(' · '),
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(fontSize: 12.5, color: r.state == 'succeeded' ? c.muted : c.warning),
                    ),
                    trailing: manage && identical(r, rollbackable) && overview.active == null
                        ? TextButton(onPressed: () => _rollback(context, ref, r), child: Text(l.updateRollBack2))
                        : null,
                    childrenPadding: const EdgeInsets.only(left: 28, bottom: 12),
                    children: [
                      for (final s in r.steps) RunStepRow(step: s),
                      if (r.log.isNotEmpty) ...[const SizedBox(height: 8), RunLog(log: r.log)],
                    ],
                  ),
              ],
            ),
    );
  }
}

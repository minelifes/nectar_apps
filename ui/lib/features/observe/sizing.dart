import 'package:material_ui/material_ui.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:nectar_apps_ui/api/session.dart';

import '../../core/format.dart';
import '../../generated/l10n.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';

/// Right-sizing of one app (GET /apps/{id}/sizing) or a row of GET /server/sizing.
class SizingReport {
  const SizingReport({
    required this.ready,
    required this.dataHours,
    required this.suggestCpu,
    required this.suggestMemory,
    required this.savingCpu,
    required this.savingMemory,
    required this.hints,
    this.appId = 0,
    this.app = '',
    this.project = '',
  });

  final bool ready;
  final double dataHours;
  final int suggestCpu;
  final int suggestMemory;
  final int savingCpu; // all replicas; negative = needs more
  final int savingMemory;
  final List<(String kind, String message)> hints;
  final int appId;
  final String app;
  final String project;

  bool get needsMore => savingMemory < 0 || savingCpu < 0;

  factory SizingReport.fromJson(Map m) {
    int n(String k) => (m[k] as num?)?.toInt() ?? 0;
    return SizingReport(
      ready: m['ready'] == true,
      dataHours: (m['data_hours'] as num?)?.toDouble() ?? 0,
      suggestCpu: n('suggest_cpu_m'),
      suggestMemory: n('suggest_memory_mb'),
      savingCpu: n('saving_cpu_m'),
      savingMemory: n('saving_memory_mb'),
      hints: [
        for (final h in (m['hints'] as List? ?? const []))
          ('${(h as Map)['kind']}', '${h['message']}'),
      ],
      appId: n('app_id'),
      app: '${m['app'] ?? ''}',
      project: '${m['project'] ?? ''}',
    );
  }
}

final appSizingProvider = FutureProvider.autoDispose.family<SizingReport, int>((
  ref,
  appId,
) async {
  final res = await ref.watch(apiProvider).get('/apps/$appId/sizing');
  return SizingReport.fromJson(res as Map);
});

final serverSizingProvider = FutureProvider.autoDispose<Map>((ref) async {
  return (await ref.watch(apiProvider).get('/server/sizing')) as Map;
});

/// Under an app's resources: what it really uses, with a button that fills in the suggestion.
class SizingHint extends ConsumerWidget {
  const SizingHint({super.key, required this.appId, this.onApply});

  final int appId;

  /// Fills the suggestion into the form (cpu millicores, memory MiB); null = can't edit.
  final void Function(int cpu, int memory)? onApply;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final rep = ref.watch(appSizingProvider(appId)).value;
    if (rep == null) return const SizedBox.shrink();
    final muted = TextStyle(color: c.muted, fontSize: 12.5);
    if (!rep.ready) {
      return Padding(
        padding: const EdgeInsets.only(top: 10),
        child: Text(
          s.sizingCollecting('${rep.dataHours.round()}'),
          style: muted,
        ),
      );
    }
    if (rep.hints.isEmpty) {
      return Padding(
        padding: const EdgeInsets.only(top: 10),
        child: Text(s.sizingFine, style: muted),
      );
    }
    final color = rep.needsMore ? c.warning : c.running;
    return Container(
      margin: const EdgeInsets.only(top: 12),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.08),
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: color.withValues(alpha: 0.3)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          for (final (_, msg) in rep.hints)
            Padding(
              padding: const EdgeInsets.only(bottom: 4),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Icon(
                    rep.needsMore
                        ? Icons.warning_amber_rounded
                        : Icons.savings_outlined,
                    size: 16,
                    color: color,
                  ),
                  const SizedBox(width: 8),
                  Expanded(
                    child: Text(msg, style: const TextStyle(fontSize: 13)),
                  ),
                ],
              ),
            ),
          if (onApply != null)
            Align(
              alignment: Alignment.centerLeft,
              child: TextButton(
                onPressed: () => onApply!(rep.suggestCpu, rep.suggestMemory),
                child: Text(
                  s.sizingApply(
                    formatCpu(rep.suggestCpu),
                    formatMb(rep.suggestMemory),
                  ),
                ),
              ),
            ),
          Text(s.sizingBasis, style: muted),
        ],
      ),
    );
  }
}

/// Server: apps that reserve far more than they use, or too little.
class SizingCard extends ConsumerWidget {
  const SizingCard({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final data = ref.watch(serverSizingProvider);
    return NCard(
      title: s.sizingTitle,
      child: AsyncBlock(
        value: data,
        loadingHeight: 80,
        onRetry: () => ref.invalidate(serverSizingProvider),
        data: (m) {
          final apps = [
            for (final a in (m['apps'] as List? ?? const []))
              SizingReport.fromJson(a as Map),
          ];
          int n(String k) => (m[k] as num?)?.toInt() ?? 0;
          final muted = TextStyle(color: c.muted, fontSize: 13);
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(
                s.sizingSummary(
                  formatCpu(n('requested_cpu_m')),
                  formatMb(n('requested_memory_mb')),
                ),
                style: muted,
              ),
              if (n('saving_memory_mb') > 0 || n('saving_cpu_m') > 0)
                Padding(
                  padding: const EdgeInsets.only(top: 4),
                  child: Text(
                    s.sizingSaving(
                      formatMb(n('saving_memory_mb')),
                      formatCpu(n('saving_cpu_m')),
                    ),
                    style: TextStyle(color: c.running, fontSize: 13),
                  ),
                ),
              const SizedBox(height: NectarTokens.gap / 2),
              if (apps.isEmpty)
                Text(
                  n('collecting') > 0
                      ? s.sizingNoneYet('${n('collecting')}')
                      : s.sizingAllFine,
                  style: muted,
                )
              else
                for (var i = 0; i < apps.length; i++) ...[
                  if (i > 0) const Divider(height: 1),
                  ListTile(
                    contentPadding: EdgeInsets.zero,
                    leading: Icon(
                      apps[i].needsMore
                          ? Icons.warning_amber_rounded
                          : Icons.savings_outlined,
                      color: apps[i].needsMore ? c.warning : c.running,
                    ),
                    title: Text('${apps[i].project} / ${apps[i].app}'),
                    subtitle: Text(
                      apps[i].hints.map((h) => h.$2).join('\n'),
                      style: const TextStyle(fontSize: 12.5),
                    ),
                    onTap: () => context.go('/apps/${apps[i].appId}'),
                  ),
                ],
            ],
          );
        },
      ),
    );
  }
}

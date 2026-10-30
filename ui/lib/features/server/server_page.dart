import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../theme/tokens.dart';
import '../../widgets/chart.dart';
import '../../widgets/common.dart';
import '../../widgets/gauge.dart';
import '../observe/event_list.dart';
import 'add_node_dialog.dart';
import 'disk_card.dart';
import 'node_card.dart';
import 'volumes_card.dart';

/// Nodes with live usage, usage history (CPU, memory, network, disk) and recent warnings.
class ServerPage extends ConsumerStatefulWidget {
  const ServerPage({super.key});

  @override
  ConsumerState<ServerPage> createState() => _ServerPageState();
}

class _ServerPageState extends ConsumerState<ServerPage> {
  String _range = '1h';

  Future<void> _refresh() async {
    ref.invalidate(nodesProvider);
    ref.invalidate(serverMetricsProvider(_range));
    ref.invalidate(serverEventsProvider);
    ref.invalidate(serverDiskProvider);
  }

  @override
  Widget build(BuildContext context) {
    final canView = ref.watch(sessionProvider.select((s) => s.can('server.view')));
    if (!canView) {
      return const PageScaffold(
        title: 'Server',
        children: [
          NCard(
            child: EmptyState(
              icon: Icons.lock_outline_rounded,
              title: 'No access',
              message: 'Your role does not include viewing the server (server.view).',
            ),
          ),
        ],
      );
    }
    final nodes = ref.watch(nodesProvider);
    final history = ref.watch(serverMetricsProvider(_range));
    final events = ref.watch(serverEventsProvider);
    return PageScaffold(
      title: 'Server',
      subtitle: 'Cluster nodes, their resources and recent warnings.',
      onRefresh: _refresh,
      actions: [
        OutlinedButton.icon(onPressed: () => context.go('/gateway'), icon: const Icon(Icons.alt_route_rounded, size: 18), label: const Text('Gateway')),
        if (ref.watch(sessionProvider).can('server.manage'))
          OutlinedButton.icon(
            onPressed: () => showDialog<void>(context: context, builder: (_) => const AddNodeDialog()),
            icon: const Icon(Icons.add_to_queue_rounded, size: 18),
            label: const Text('Add node'),
          ),
        RangePicker(value: _range, options: usageRanges, onChanged: (r) => setState(() => _range = r)),
      ],
      children: [
        AsyncBlock(
          value: nodes,
          onRetry: _refresh,
          data: (list) => Column(
            children: [
              for (final n in list) ...[
                NodeCard(node: n, multiNode: list.length > 1 || n.unschedulable),
                const SizedBox(height: NectarTokens.gap),
                if (history.hasValue)
                  for (final h in history.value!.where((h) => h.name == n.name)) ...[
                    _NodeHistoryCard(history: h, range: _range),
                    const SizedBox(height: NectarTokens.gap),
                  ],
              ],
            ],
          ),
        ),
        const DiskCard(),
        const SizedBox(height: NectarTokens.gap),
        const VolumesCard(),
        const SizedBox(height: NectarTokens.gap),
        NCard(
          title: 'Warnings',
          child: AsyncBlock(
            value: events,
            loadingHeight: 80,
            data: (list) => EventList(events: list, showNamespace: true, empty: 'No warnings. All quiet.'),
          ),
        ),
      ],
    );
  }
}

class _NodeHistoryCard extends StatelessWidget {
  const _NodeHistoryCard({required this.history, required this.range});

  final NodeHistory history;
  final String range;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final pts = history.points;
    final last = pts.isEmpty ? null : pts.last;
    final cpu = ChartSeries(
      label: 'CPU',
      color: NectarTokens.amber,
      fill: true,
      points: [for (final p in pts) (p.t, p.cpuM.toDouble())],
    );
    final mem = ChartSeries(
      label: 'Memory',
      color: c.deploying,
      fill: true,
      points: [for (final p in pts) (p.t, p.memB.toDouble())],
    );
    final rx = ChartSeries(label: 'In', color: c.running, points: [for (final p in pts) (p.t, p.rxBps.toDouble())]);
    final tx = ChartSeries(label: 'Out', color: NectarTokens.amber, points: [for (final p in pts) (p.t, p.txBps.toDouble())]);
    final charts = [
      ('CPU', TimeSeriesChart(series: [cpu], formatY: (v) => formatCpu(v.round()), yMax: history.cpuAllocM.toDouble())),
      ('Memory', TimeSeriesChart(series: [mem], formatY: (v) => formatBytes(v.round()), yMax: history.memAllocB.toDouble())),
      ('Network', TimeSeriesChart(series: [rx, tx], formatY: (v) => '${formatBytes(v.round())}/s')),
    ];
    return NCard(
      title: 'History · ${history.name} · last $range',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          LayoutBuilder(
            builder: (context, box) {
              final cols = box.maxWidth >= 1000 ? 3 : (box.maxWidth >= 640 ? 2 : 1);
              final w = (box.maxWidth - (cols - 1) * 20) / cols;
              return Wrap(
                spacing: 20,
                runSpacing: 20,
                children: [
                  for (final (title, chart) in charts)
                    SizedBox(
                      width: w,
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [Text(title, style: Theme.of(context).textTheme.titleSmall), const SizedBox(height: 8), chart],
                      ),
                    ),
                ],
              );
            },
          ),
          if (last != null && last.diskTotal > 0) ...[
            const SizedBox(height: 20),
            UsageBar(
              label: 'Disk',
              value: last.diskUsed / last.diskTotal,
              trailing: '${formatBytes(last.diskUsed)} of ${formatBytes(last.diskTotal)}',
            ),
          ],
        ],
      ),
    );
  }
}

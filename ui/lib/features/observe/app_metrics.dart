import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/models.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/chart.dart';
import '../../widgets/common.dart';

/// Resource usage of an app (with its requests as dashed lines) and, for web apps behind the gateway,
/// request traffic and the latest requests.
class AppMetrics extends ConsumerStatefulWidget {
  const AppMetrics({super.key, required this.app, required this.canLogs});

  final App app;
  final bool canLogs;

  @override
  ConsumerState<AppMetrics> createState() => _AppMetricsState();
}

class _AppMetricsState extends ConsumerState<AppMetrics> {
  String _range = '1h';

  @override
  Widget build(BuildContext context) {
    final id = widget.app.id;
    final usage = ref.watch(appUsageProvider((id, _range)));
    final c = NectarColors.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        NCard(
          title: 'Resources',
          trailing: RangePicker(value: _range, options: usageRanges, onChanged: (r) => setState(() => _range = r)),
          child: AsyncBlock(
            value: usage,
            onRetry: () => ref.invalidate(appUsageProvider((id, _range))),
            data: (u) {
              final pts = u.points;
              List<(DateTime, double)> flat(double v) => pts.isEmpty ? const [] : [(pts.first.t, v), (pts.last.t, v)];
              final cpu = TimeSeriesChart(
                formatY: (v) => formatCpu(v.round()),
                series: [
                  ChartSeries(label: 'CPU', color: NectarTokens.amber, fill: true, points: [for (final p in pts) (p.t, p.cpuM.toDouble())]),
                  if (u.requestCpuM > 0)
                    ChartSeries(label: 'Requested', color: c.muted, dashed: true, points: flat(u.requestCpuM.toDouble())),
                ],
              );
              final mem = TimeSeriesChart(
                formatY: (v) => formatBytes(v.round()),
                series: [
                  ChartSeries(label: 'Memory', color: c.deploying, fill: true, points: [for (final p in pts) (p.t, p.memB.toDouble())]),
                  if (u.requestMemB > 0)
                    ChartSeries(label: 'Requested', color: c.muted, dashed: true, points: flat(u.requestMemB.toDouble())),
                ],
              );
              return Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  _Grid(children: [('CPU', cpu), ('Memory', mem)]),
                  if (u.replicas > 1)
                    Padding(
                      padding: const EdgeInsets.only(top: 12),
                      child: Text(
                        'Totals across ${u.replicas} replicas; the dashed line is what all of them requested together.',
                        style: TextStyle(color: c.muted, fontSize: 12.5),
                      ),
                    ),
                ],
              );
            },
          ),
        ),
        if (widget.app.isWeb) ...[
          const SizedBox(height: NectarTokens.gap),
          _Traffic(app: widget.app),
          if (widget.canLogs) ...[
            const SizedBox(height: NectarTokens.gap),
            _AccessLog(app: widget.app),
          ],
        ],
      ],
    );
  }
}

class _Grid extends StatelessWidget {
  const _Grid({required this.children});

  final List<(String, Widget)> children;

  @override
  Widget build(BuildContext context) => LayoutBuilder(
        builder: (context, box) {
          final cols = box.maxWidth >= 760 ? 2 : 1;
          final w = (box.maxWidth - (cols - 1) * 20) / cols;
          return Wrap(
            spacing: 20,
            runSpacing: 20,
            children: [
              for (final (title, chart) in children)
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
      );
}

class _Traffic extends ConsumerWidget {
  const _Traffic({required this.app});

  final App app;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final traffic = ref.watch(appTrafficProvider(app.id));
    final c = NectarColors.of(context);
    return NCard(
      title: 'Traffic · last hour',
      trailing: IconButton(
        tooltip: 'Refresh',
        icon: const Icon(Icons.refresh_rounded, size: 18),
        onPressed: () => ref.invalidate(appTrafficProvider(app.id)),
      ),
      child: AsyncBlock(
        value: traffic,
        loadingHeight: 100,
        onRetry: () => ref.invalidate(appTrafficProvider(app.id)),
        data: (domains) {
          if (domains.isEmpty) {
            return Text(
              'No requests yet. Traffic appears once the app has a domain and the gateway routes requests to it.',
              style: TextStyle(color: c.muted, fontSize: 13.5),
            );
          }
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              for (final d in domains) ...[
                if (domains.length > 1)
                  Padding(padding: const EdgeInsets.only(bottom: 8), child: Text(d.domain, style: monoStyle(context, size: 13))),
                Wrap(
                  spacing: 28,
                  runSpacing: 10,
                  children: [
                    _Stat(label: 'Requests', value: '${d.requests}'),
                    _Stat(label: 'Per second', value: d.rps.toStringAsFixed(d.rps < 10 ? 2 : 1)),
                    _Stat(
                      label: 'Errors (5xx)',
                      value: '${(d.errorRate * 100).toStringAsFixed(1)}%',
                      color: d.errorRate >= 0.05 ? c.failed : (d.errorRate > 0 ? c.warning : null),
                    ),
                    _Stat(label: 'p50', value: '${d.p50} ms'),
                    _Stat(label: 'p95', value: '${d.p95} ms'),
                    _Stat(label: 'p99', value: '${d.p99} ms'),
                  ],
                ),
                if (d.status.isNotEmpty) ...[
                  const SizedBox(height: 10),
                  Text(
                    (d.status.entries.toList()..sort((a, b) => a.key.compareTo(b.key))).map((e) => '${e.key}: ${e.value}').join('   '),
                    style: TextStyle(color: c.muted, fontSize: 12.5),
                  ),
                ],
                if (d.series.isNotEmpty) ...[
                  const SizedBox(height: 16),
                  _Grid(children: [
                    (
                      'Requests per minute',
                      TimeSeriesChart(
                        height: 130,
                        formatY: (v) => v.round().toString(),
                        series: [
                          ChartSeries(label: 'Requests', color: NectarTokens.amber, fill: true, points: [for (final p in d.series) (p.$1, p.$2.toDouble())]),
                          ChartSeries(label: '5xx', color: c.failed, points: [for (final p in d.series) (p.$1, p.$3.toDouble())]),
                        ],
                      ),
                    ),
                    (
                      'Latency p95',
                      TimeSeriesChart(
                        height: 130,
                        formatY: (v) => '${v.round()} ms',
                        series: [
                          ChartSeries(label: 'p95', color: c.deploying, points: [for (final p in d.series) (p.$1, p.$4.toDouble())]),
                        ],
                      ),
                    ),
                  ]),
                ],
                if (d != domains.last) const Divider(height: 32),
              ],
            ],
          );
        },
      ),
    );
  }
}

class _Stat extends StatelessWidget {
  const _Stat({required this.label, required this.value, this.color});

  final String label;
  final String value;
  final Color? color;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(label, style: TextStyle(color: c.muted, fontSize: 12)),
        const SizedBox(height: 2),
        Text(value, style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600, color: color ?? c.text)),
      ],
    );
  }
}

class _AccessLog extends ConsumerWidget {
  const _AccessLog({required this.app});

  final App app;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final log = ref.watch(appAccessLogProvider(app.id));
    final c = NectarColors.of(context);
    return NCard(
      title: 'Latest requests',
      trailing: IconButton(
        tooltip: 'Refresh',
        icon: const Icon(Icons.refresh_rounded, size: 18),
        onPressed: () => ref.invalidate(appAccessLogProvider(app.id)),
      ),
      child: AsyncBlock(
        value: log,
        loadingHeight: 100,
        data: (entries) {
          if (entries.isEmpty) return Text('No requests yet.', style: TextStyle(color: c.muted, fontSize: 13.5));
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              for (final e in entries)
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 3),
                  child: Row(
                    children: [
                      SizedBox(
                        width: 64,
                        child: Text(e.time == null ? '' : timeAgo(e.time), style: TextStyle(color: c.muted, fontSize: 12)),
                      ),
                      SizedBox(
                        width: 40,
                        child: Text(
                          '${e.status}',
                          style: monoStyle(
                            context,
                            size: 12.5,
                            color: e.status >= 500 ? c.failed : (e.status >= 400 ? c.warning : c.running),
                          ),
                        ),
                      ),
                      SizedBox(width: 58, child: Text(e.method, style: monoStyle(context, size: 12.5))),
                      Expanded(
                        child: Text(e.path, maxLines: 1, overflow: TextOverflow.ellipsis, style: monoStyle(context, size: 12.5)),
                      ),
                      SizedBox(
                        width: 70,
                        child: Text('${e.ms} ms', textAlign: TextAlign.right, style: TextStyle(color: c.muted, fontSize: 12)),
                      ),
                      SizedBox(
                        width: 120,
                        child: Text(e.ip, textAlign: TextAlign.right, style: TextStyle(color: c.muted, fontSize: 12)),
                      ),
                    ],
                  ),
                ),
            ],
          );
        },
      ),
    );
  }
}

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';

/// One vulnerability of a scan.
class ScanFinding {
  const ScanFinding({
    required this.id,
    required this.severity,
    required this.package,
    required this.installed,
    required this.fixed,
    required this.target,
    required this.title,
  });

  final String id;
  final String severity; // CRITICAL | HIGH | MEDIUM | LOW | UNKNOWN
  final String package;
  final String installed;
  final String fixed; // '' = no fix yet
  final String target;
  final String title;

  factory ScanFinding.fromJson(Map m) => ScanFinding(
    id: '${m['id'] ?? ''}',
    severity: '${m['severity'] ?? ''}',
    package: '${m['package'] ?? ''}',
    installed: '${m['installed'] ?? ''}',
    fixed: '${m['fixed'] ?? ''}',
    target: '${m['target'] ?? ''}',
    title: '${m['title'] ?? ''}',
  );
}

/// The last vulnerability scan of an app's image (GET /apps/{id}/scan).
class ImageScan {
  const ImageScan({
    required this.status,
    required this.image,
    required this.currentImage,
    required this.message,
    required this.stale,
    required this.total,
    required this.critical,
    required this.high,
    required this.medium,
    required this.low,
    required this.unknown,
    required this.fixable,
    required this.findings,
    required this.finishedAt,
  });

  final String status; // none | running | done | failed
  final String image;
  final String currentImage; // '' = not deployed
  final String message;
  final bool stale; // the app runs another image now
  final int total;
  final int critical;
  final int high;
  final int medium;
  final int low;
  final int unknown;
  final int fixable;
  final List<ScanFinding> findings;
  final DateTime? finishedAt;

  bool get running => status == 'running';

  factory ImageScan.fromJson(Map m) {
    int n(String k) => (m[k] as num?)?.toInt() ?? 0;
    return ImageScan(
      status: '${m['status'] ?? 'none'}',
      image: '${m['image'] ?? ''}',
      currentImage: '${m['current_image'] ?? ''}',
      message: '${m['message'] ?? ''}',
      stale: m['stale'] == true,
      total: n('total'),
      critical: n('critical'),
      high: n('high'),
      medium: n('medium'),
      low: n('low'),
      unknown: n('unknown'),
      fixable: n('fixable'),
      findings: [
        for (final f in (m['findings'] as List? ?? const []))
          ScanFinding.fromJson(f as Map),
      ],
      finishedAt: DateTime.tryParse('${m['finished_at'] ?? ''}')?.toLocal(),
    );
  }
}

final imageScanProvider = FutureProvider.autoDispose.family<ImageScan, int>((
  ref,
  appId,
) async {
  final res = await ref.watch(apiProvider).get('/apps/$appId/scan');
  return ImageScan.fromJson(res as Map);
});

/// Critical vulnerabilities per app of a project (GET /projects/{id}/scans), for the app tiles.
final projectCriticalProvider = FutureProvider.autoDispose
    .family<Map<int, int>, int>((ref, projectId) async {
      final res = await ref
          .watch(apiProvider)
          .get('/projects/$projectId/scans');
      return {
        for (final m in (res as List? ?? const []))
          if (m is Map && m['status'] == 'done')
            (m['app_id'] as num).toInt(): (m['critical'] as num?)?.toInt() ?? 0,
      };
    });

Color _severityColor(NectarColors c, String severity) => switch (severity) {
  'CRITICAL' => c.failed,
  'HIGH' => c.warning,
  'MEDIUM' => c.deploying,
  _ => c.stopped,
};

/// App → Security: known vulnerabilities in the image the app runs.
class ImageScanCard extends ConsumerStatefulWidget {
  const ImageScanCard({super.key, required this.app});

  final App app;

  @override
  ConsumerState<ImageScanCard> createState() => _ImageScanCardState();
}

class _ImageScanCardState extends ConsumerState<ImageScanCard> {
  Timer? _poll;
  bool _starting = false;
  bool _fixableOnly = false;

  @override
  void dispose() {
    _poll?.cancel();
    super.dispose();
  }

  Future<void> _scan() async {
    setState(() => _starting = true);
    try {
      await ref.read(apiProvider).post('/apps/${widget.app.id}/scan');
      ref.invalidate(imageScanProvider(widget.app.id));
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _starting = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final id = widget.app.id;
    final canScan = ref.watch(
      sessionProvider.select(
        (x) => x.can('app.deploy', projectId: widget.app.projectId),
      ),
    );
    final scan = ref.watch(imageScanProvider(id));
    // refresh while the scan runs
    final running = scan.value?.running ?? false;
    if (running && _poll == null) {
      _poll = Timer.periodic(
        const Duration(seconds: 4),
        (_) => ref.invalidate(imageScanProvider(id)),
      );
    } else if (!running && _poll != null) {
      _poll!.cancel();
      _poll = null;
    }
    return AsyncBlock(
      value: scan,
      loadingHeight: 120,
      onRetry: () => ref.invalidate(imageScanProvider(id)),
      data: (sc) {
        final button = canScan && sc.currentImage.isNotEmpty
            ? FilledButton.tonalIcon(
                onPressed: sc.running || _starting ? null : _scan,
                icon: const Icon(Icons.shield_outlined, size: 18),
                label: Text(sc.status == 'none' ? s.scanNow : s.scanAgain),
              )
            : null;
        final muted = TextStyle(color: c.muted, fontSize: 13);
        final shown = [
          for (final f in sc.findings)
            if (!_fixableOnly || f.fixed.isNotEmpty) f,
        ];
        return NCard(
          title: s.scanTitle,
          trailing: button,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              if (sc.currentImage.isEmpty)
                Text(s.scanNotDeployed, style: muted)
              else if (sc.status == 'none')
                Text(s.scanNever, style: muted)
              else ...[
                Text(
                  sc.image,
                  style: monoStyle(context, size: 12.5),
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                ),
                const SizedBox(height: 6),
                if (sc.running)
                  Row(
                    children: [
                      const SizedBox(
                        width: 16,
                        height: 16,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      ),
                      const SizedBox(width: 10),
                      Flexible(child: Text(s.scanRunning, style: muted)),
                    ],
                  )
                else if (sc.status == 'failed') ...[
                  Text(s.scanFailed, style: TextStyle(color: c.failed)),
                  const SizedBox(height: 4),
                  SelectableText(
                    sc.message,
                    style: monoStyle(context, size: 12.5),
                  ),
                ] else ...[
                  Text(
                    [
                      if (sc.finishedAt != null) timeAgo(sc.finishedAt),
                      if (sc.total == 0)
                        s.scanClean
                      else ...[
                        s.scanFound('${sc.total}'),
                        s.scanHaveFix('${sc.fixable}'),
                      ],
                    ].join(' · '),
                    style: muted,
                  ),
                  if (sc.stale)
                    Padding(
                      padding: const EdgeInsets.only(top: 6),
                      child: Text(
                        s.scanStale,
                        style: TextStyle(color: c.warning, fontSize: 13),
                      ),
                    ),
                  if (sc.total > 0) ...[
                    const SizedBox(height: 12),
                    Wrap(
                      spacing: 8,
                      runSpacing: 8,
                      crossAxisAlignment: WrapCrossAlignment.center,
                      children: [
                        for (final (label, n, sev) in [
                          (s.scanCritical, sc.critical, 'CRITICAL'),
                          (s.scanHigh, sc.high, 'HIGH'),
                          (s.scanMedium, sc.medium, 'MEDIUM'),
                          (s.scanLow, sc.low + sc.unknown, 'LOW'),
                        ])
                          _Count(
                            label: label,
                            count: n,
                            color: _severityColor(c, sev),
                          ),
                        FilterChip(
                          label: Text(s.scanFixableOnly),
                          selected: _fixableOnly,
                          onSelected: (v) => setState(() => _fixableOnly = v),
                        ),
                      ],
                    ),
                    const SizedBox(height: 8),
                    for (var i = 0; i < shown.length; i++) ...[
                      if (i > 0) const Divider(height: 1),
                      _FindingRow(finding: shown[i]),
                    ],
                    if (sc.total > sc.findings.length)
                      Padding(
                        padding: const EdgeInsets.only(top: 8),
                        child: Text(
                          s.scanMore('${sc.total - sc.findings.length}'),
                          style: muted,
                        ),
                      ),
                  ],
                ],
              ],
            ],
          ),
        );
      },
    );
  }
}

class _Count extends StatelessWidget {
  const _Count({required this.label, required this.count, required this.color});

  final String label;
  final int count;
  final Color color;

  @override
  Widget build(BuildContext context) {
    final c = count > 0 ? color : NectarColors.of(context).stopped;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
      decoration: BoxDecoration(
        color: c.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: c.withValues(alpha: 0.28)),
      ),
      child: Text(
        '$count $label',
        style: TextStyle(color: c, fontSize: 12.5, fontWeight: FontWeight.w600),
      ),
    );
  }
}

class _FindingRow extends StatelessWidget {
  const _FindingRow({required this.finding});

  final ScanFinding finding;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final f = finding;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            width: 8,
            height: 8,
            margin: const EdgeInsets.only(top: 5, right: 10),
            decoration: BoxDecoration(
              color: _severityColor(c, f.severity),
              shape: BoxShape.circle,
            ),
          ),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                SelectableText(
                  '${f.id}  ${f.package} ${f.installed}',
                  style: monoStyle(context, size: 12.5),
                ),
                if (f.title.isNotEmpty)
                  Text(
                    f.title,
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(color: c.muted, fontSize: 12.5),
                  ),
                Text(
                  [
                    f.fixed.isEmpty ? s.scanNoFix : s.scanFixedIn(f.fixed),
                    if (f.target.isNotEmpty) f.target,
                  ].join(' · '),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                    color: f.fixed.isEmpty ? c.muted : c.running,
                    fontSize: 12.5,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

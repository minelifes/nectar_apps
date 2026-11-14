import 'package:material_ui/material_ui.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:nectar_apps_ui/theme/tokens.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../widgets/common.dart';

/// GET /apps/{id}/image-updates.
class ImageWatchInfo {
  const ImageWatchInfo({
    required this.policy,
    required this.supported,
    required this.reason,
    required this.status,
    required this.message,
    required this.running,
    required this.latest,
    required this.seenAt,
    required this.checkedAt,
  });

  final String policy; // '' | notify | auto
  final bool supported;
  final String reason;
  final String
  status; // '' (never checked) | current | update | error | unknown
  final String message;
  final String running;
  final String latest;
  final DateTime? seenAt;
  final DateTime? checkedAt;

  factory ImageWatchInfo.fromJson(Map m) {
    final w = (m['watch'] as Map?) ?? const {};
    DateTime? t(Object? v) => DateTime.tryParse('${v ?? ''}')?.toLocal();
    return ImageWatchInfo(
      policy: '${m['policy'] ?? ''}',
      supported: m['supported'] == true,
      reason: '${m['reason'] ?? ''}',
      status: '${w['status'] ?? ''}',
      message: '${w['message'] ?? ''}',
      running: '${w['running_digest'] ?? ''}',
      latest: '${w['latest_digest'] ?? ''}',
      seenAt: t(w['seen_at']),
      checkedAt: t(w['checked_at']),
    );
  }
}

final imageWatchProvider = FutureProvider.autoDispose
    .family<ImageWatchInfo, int>((ref, appId) async {
      final res = await ref
          .watch(apiProvider)
          .get('/apps/$appId/image-updates');
      return ImageWatchInfo.fromJson(res as Map);
    });

String _short(String digest) {
  final d = digest.replaceFirst('sha256:', '');
  return d.length > 12 ? d.substring(0, 12) : d;
}

/// App → Settings: follow the image tag (off / notify / deploy automatically).
class ImageUpdatesCard extends ConsumerStatefulWidget {
  const ImageUpdatesCard({super.key, required this.app});

  final App app;

  @override
  ConsumerState<ImageUpdatesCard> createState() => _ImageUpdatesCardState();
}

class _ImageUpdatesCardState extends ConsumerState<ImageUpdatesCard> {
  bool _busy = false;

  Future<void> _run(Future<void> Function() f) async {
    setState(() => _busy = true);
    try {
      await f();
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _setPolicy(String policy) => _run(() async {
    await ref
        .read(apiProvider)
        .patch(
          '/apps/${widget.app.id}',
          data: {
            'config': {'image_updates': policy},
          },
        );
    ref.invalidate(imageWatchProvider(widget.app.id));
    ref.invalidate(appProvider(widget.app.id));
  });

  Future<void> _check() => _run(() async {
    await ref
        .read(apiProvider)
        .post('/apps/${widget.app.id}/image-updates/check');
    ref.invalidate(imageWatchProvider(widget.app.id));
  });

  Future<void> _deploy() => _run(() async {
    await ref
        .read(apiProvider)
        .post('/apps/${widget.app.id}/image-updates/deploy');
    ref.invalidate(imageWatchProvider(widget.app.id));
    ref.invalidate(appProvider(widget.app.id));
    if (mounted) {
      showSnack(context, S.of(context).imageUpdatesDeploying);
    }
  });

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final id = widget.app.id;
    final session = ref.watch(sessionProvider);
    final canEdit = session.can('app.edit', projectId: widget.app.projectId);
    final canDeploy = session.can(
      'app.deploy',
      projectId: widget.app.projectId,
    );
    final info = ref.watch(imageWatchProvider(id));
    final muted = TextStyle(color: c.muted, fontSize: 13);
    return NCard(
      title: s.imageUpdatesTitle,
      child: AsyncBlock(
        value: info,
        onRetry: () => ref.invalidate(imageWatchProvider(id)),
        data: (w) {
          if (!w.supported) return Text(w.reason, style: muted);
          final String state = switch (w.status) {
            'current' => s.imageUpdatesCurrent,
            'update' => s.imageUpdatesNewer(_short(w.latest)),
            'unknown' => s.imageUpdatesUnknown,
            'error' => w.message,
            _ => s.imageUpdatesNeverChecked,
          };
          final color = switch (w.status) {
            'update' => c.warning,
            'error' => c.failed,
            'current' => c.running,
            _ => c.muted,
          };
          return Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(s.imageUpdatesHelp, style: muted),
              const SizedBox(height: 10),
              SegmentedButton<String>(
                segments: [
                  ButtonSegment(value: '', label: Text(s.imageUpdatesOff)),
                  ButtonSegment(
                    value: 'notify',
                    label: Text(s.imageUpdatesNotify),
                  ),
                  ButtonSegment(value: 'auto', label: Text(s.imageUpdatesAuto)),
                ],
                selected: {w.policy},
                onSelectionChanged: canEdit && !_busy
                    ? (v) => _setPolicy(v.first)
                    : null,
              ),
              const SizedBox(height: 12),
              Text(state, style: TextStyle(color: color, fontSize: 13)),
              if (w.status == 'update' && w.message.isNotEmpty)
                Padding(
                  padding: const EdgeInsets.only(top: 4),
                  child: Text(w.message, style: muted),
                ),
              if (w.checkedAt != null)
                Padding(
                  padding: const EdgeInsets.only(top: 4),
                  child: Text(
                    [
                      s.imageUpdatesChecked(timeAgo(w.checkedAt)),
                      if (w.running.isNotEmpty)
                        s.imageUpdatesRunning(_short(w.running)),
                    ].join(' · '),
                    style: muted,
                  ),
                ),
              const SizedBox(height: 8),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  OutlinedButton.icon(
                    onPressed: _busy ? null : _check,
                    icon: const Icon(Icons.refresh_rounded, size: 18),
                    label: Text(s.imageUpdatesCheck),
                  ),
                  if (w.status == 'update' && canDeploy)
                    FilledButton.icon(
                      onPressed: _busy ? null : _deploy,
                      icon: const Icon(Icons.rocket_launch_outlined, size: 18),
                      label: Text(s.imageUpdatesDeploy),
                    ),
                ],
              ),
            ],
          );
        },
      ),
    );
  }
}

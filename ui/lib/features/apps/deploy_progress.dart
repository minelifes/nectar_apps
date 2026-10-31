import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/hex.dart';

enum _StepState { waiting, active, done, failed, skipped }

/// Live progress of one release: created → preparing (image pull, artifact) → starting → healthy → route live.
/// Polls the API every 1.5 s until the release is finished and the route settled.
class DeployProgress extends ConsumerStatefulWidget {
  const DeployProgress({
    super.key,
    required this.app,
    required this.releaseId,
    this.onFinished,
  });

  final App app;
  final int releaseId;

  /// Called once with true (live) or false (failed).
  final void Function(bool ok)? onFinished;

  @override
  ConsumerState<DeployProgress> createState() => _DeployProgressState();
}

class _DeployProgressState extends ConsumerState<DeployProgress> {
  Timer? _timer;
  Release? _release;
  AppLiveStatus? _live;
  List<Domain> _domains = const [];
  String? _error;
  bool _finished = false;
  DateTime? _liveAt;

  @override
  void initState() {
    super.initState();
    _poll();
    _timer = Timer.periodic(const Duration(milliseconds: 1500), (_) => _poll());
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  Future<void> _poll() async {
    final api = ref.read(apiProvider);
    final id = widget.app.id;
    try {
      final results = await Future.wait([
        api.get('/apps/$id/releases'),
        api
            .get('/apps/$id/status')
            .then<Object?>((v) => v, onError: (Object _) => null),
        if (widget.app.isWeb)
          api
              .get('/apps/$id/domains')
              .then<Object?>((v) => v, onError: (Object _) => null),
      ]);
      if (!mounted) return;
      final rels = listOf(results[0], Release.fromJson);
      final rel = rels.where((r) => r.id == widget.releaseId).firstOrNull;
      setState(() {
        _error = null;
        _release = rel;
        if (results[1] != null) _live = AppLiveStatus.fromJson(results[1]);
        if (results.length > 2 && results[2] != null) {
          _domains = listOf(results[2], Domain.fromJson);
        }
      });
      _checkFinished();
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    }
  }

  void _checkFinished() {
    final rel = _release;
    if (rel == null || _finished) return;
    if (rel.status == 'failed') {
      _finish(false);
    } else if (rel.status == 'live' || rel.status == 'superseded') {
      _liveAt ??= DateTime.now();
      // wait for the route (gateway sync runs right after the deploy), at most 30 s
      final routeSettled =
          !widget.app.isWeb ||
          _domains.isEmpty ||
          _domains.any((d) => d.status == 'active' || d.status == 'error') ||
          DateTime.now().difference(_liveAt!) > const Duration(seconds: 30);
      if (routeSettled) _finish(true);
    }
  }

  void _finish(bool ok) {
    _finished = true;
    _timer?.cancel();
    widget.onFinished?.call(ok);
  }

  List<PodInfo> get _releasePods {
    final rel = _release;
    final pods = _live?.pods ?? const <PodInfo>[];
    if (rel == null) return pods;
    final mine = pods.where((p) => p.release == '${rel.number}').toList();
    return mine.isEmpty ? pods : mine;
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final rel = _release;
    final pods = _releasePods;
    final failed = rel?.status == 'failed';
    final live = rel?.status == 'live' || rel?.status == 'superseded';
    final anyRunning = pods.any((p) => p.phase == 'Running');
    final problem = pods
        .map((p) => p.problem)
        .where((p) => p.isNotEmpty)
        .firstOrNull;
    final domain = _domains.where((d) => d.kind == 'app').firstOrNull;

    _StepState s(bool done, bool active) => failed && !done
        ? (active ? _StepState.failed : _StepState.waiting)
        : (done
              ? _StepState.done
              : (active ? _StepState.active : _StepState.waiting));

    final prepared = anyRunning || live;
    final steps = <(String, String?, _StepState)>[
      (
        'Release ${rel == null ? '' : '#${rel.number} '}created',
        null,
        s(rel != null, rel == null),
      ),
      (
        'Preparing',
        'pulling the image and fetching your file',
        s(prepared, rel != null && !prepared),
      ),
      ('Starting', problem, s(live, prepared && !live)),
      ('Healthy', null, s(live, false)),
      if (widget.app.isWeb)
        (
          'Route live',
          domain == null
              ? 'no domain yet: set a base domain in Settings or add one to the app'
              : (domain.status == 'active'
                    ? domain.host
                    : (domain.statusMessage.isEmpty
                          ? domain.host
                          : domain.statusMessage)),
          domain == null
              ? (live ? _StepState.skipped : _StepState.waiting)
              : (domain.status == 'active'
                    ? _StepState.done
                    : (domain.status == 'error'
                          ? _StepState.failed
                          : (live ? _StepState.active : _StepState.waiting))),
        ),
    ];

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        for (final (title, detail, state) in steps)
          _StepRow(title: title, detail: detail, state: state),
        if (failed && (rel?.message ?? '').isNotEmpty)
          Container(
            margin: const EdgeInsets.only(top: 12),
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: c.failed.withValues(alpha: 0.08),
              borderRadius: BorderRadius.circular(NectarTokens.radiusSmall),
              border: Border.all(color: c.failed.withValues(alpha: 0.3)),
            ),
            child: SelectableText(
              rel!.message,
              style: monoStyle(context, size: 12.5),
            ),
          ),
        if (_error != null)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text(
              _error!,
              style: TextStyle(color: c.muted, fontSize: 12),
            ),
          ),
      ],
    );
  }
}

class _StepRow extends StatelessWidget {
  const _StepRow({
    required this.title,
    required this.detail,
    required this.state,
  });

  final String title;
  final String? detail;
  final _StepState state;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final Widget icon = switch (state) {
      _StepState.done => Icon(
        Icons.check_circle_rounded,
        color: c.running,
        size: 22,
      ),
      _StepState.failed => Icon(
        Icons.cancel_rounded,
        color: c.failed,
        size: 22,
      ),
      _StepState.active => const SizedBox(
        width: 22,
        height: 22,
        child: Center(child: HexLoader(size: 20)),
      ),
      _StepState.skipped => Icon(
        Icons.remove_circle_outline_rounded,
        color: c.muted,
        size: 22,
      ),
      _StepState.waiting => Icon(
        Icons.radio_button_unchecked_rounded,
        color: c.border,
        size: 22,
      ),
    };
    final color = state == _StepState.waiting ? c.muted : c.text;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 7),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          icon,
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  title,
                  style: TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                    color: color,
                  ),
                ),
                if (detail != null &&
                    detail!.isNotEmpty &&
                    state != _StepState.waiting)
                  Padding(
                    padding: const EdgeInsets.only(top: 2),
                    child: Text(
                      detail!,
                      style: TextStyle(
                        fontSize: 12.5,
                        color: state == _StepState.failed ? c.failed : c.muted,
                      ),
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

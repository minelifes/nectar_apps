import 'package:flutter/material.dart';

import '../theme/tokens.dart';

/// Visual group of an app/release/domain status string from the API.
enum StatusTone { running, deploying, warning, failed, stopped }

StatusTone toneFor(String status) => switch (status) {
      'running' || 'live' || 'active' || 'ready' || 'scheduled' => StatusTone.running,
      'deploying' || 'pending' || 'waiting' || 'new' => StatusTone.deploying,
      'degraded' || 'warning' => StatusTone.warning,
      'failed' || 'error' => StatusTone.failed,
      _ => StatusTone.stopped,
    };

Color toneColor(BuildContext context, StatusTone tone) {
  final c = NectarColors.of(context);
  return switch (tone) {
    StatusTone.running => c.running,
    StatusTone.deploying => c.deploying,
    StatusTone.warning => c.warning,
    StatusTone.failed => c.failed,
    StatusTone.stopped => c.stopped,
  };
}

String statusLabel(String status) => switch (status) {
      'new' => 'Not deployed',
      '' => 'Unknown',
      _ => status[0].toUpperCase() + status.substring(1),
    };

/// A small colored dot. Pulses while the status is in progress (deploying).
class StatusDot extends StatefulWidget {
  const StatusDot({super.key, required this.status, this.size = 8});

  final String status;
  final double size;

  @override
  State<StatusDot> createState() => _StatusDotState();
}

class _StatusDotState extends State<StatusDot> with SingleTickerProviderStateMixin {
  AnimationController? _pulse;

  bool get _animated => toneFor(widget.status) == StatusTone.deploying && widget.status != 'new';

  @override
  void initState() {
    super.initState();
    _sync();
  }

  @override
  void didUpdateWidget(StatusDot old) {
    super.didUpdateWidget(old);
    _sync();
  }

  void _sync() {
    if (_animated && _pulse == null) {
      _pulse = AnimationController(vsync: this, duration: const Duration(milliseconds: 1100))..repeat(reverse: true);
    } else if (!_animated && _pulse != null) {
      _pulse!.dispose();
      _pulse = null;
    }
  }

  @override
  void dispose() {
    _pulse?.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final color = toneColor(context, toneFor(widget.status));
    final dot = Container(
      width: widget.size,
      height: widget.size,
      decoration: BoxDecoration(
        color: color,
        shape: BoxShape.circle,
        boxShadow: [BoxShadow(color: color.withValues(alpha: 0.45), blurRadius: widget.size)],
      ),
    );
    final pulse = _pulse;
    if (pulse == null) return dot;
    return FadeTransition(opacity: Tween(begin: 0.35, end: 1.0).animate(pulse), child: dot);
  }
}

/// Status pill: dot + label on a tinted background.
class StatusPill extends StatelessWidget {
  const StatusPill({super.key, required this.status, this.label});

  final String status;
  final String? label;

  @override
  Widget build(BuildContext context) {
    final color = toneColor(context, toneFor(status));
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 9, vertical: 4),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: color.withValues(alpha: 0.28)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          StatusDot(status: status, size: 7),
          const SizedBox(width: 6),
          Text(
            label ?? statusLabel(status),
            style: TextStyle(fontSize: 12, fontWeight: FontWeight.w600, color: color, height: 1.2),
          ),
        ],
      ),
    );
  }
}

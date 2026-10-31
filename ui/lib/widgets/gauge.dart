import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../theme/tokens.dart';

/// Ring gauge for a 0–1 ratio with a big number in the middle (CPU, memory, quota).
class RingGauge extends StatelessWidget {
  const RingGauge({
    super.key,
    required this.value,
    required this.label,
    this.caption,
    this.size = 120,
    this.unavailable = false,
  });

  /// 0–1 (clamped). Ignored when [unavailable].
  final double value;
  final String label;
  final String? caption;
  final double size;
  final bool unavailable;

  Color _color(NectarColors c) {
    if (unavailable) return c.stopped;
    if (value >= 0.9) return c.failed;
    if (value >= 0.75) return c.warning;
    return NectarTokens.amber;
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final v = unavailable ? 0.0 : value.clamp(0.0, 1.0);
    final theme = Theme.of(context);
    return Semantics(
      label:
          '$label ${unavailable ? 'unavailable' : '${(v * 100).round()} percent'}',
      child: SizedBox.square(
        dimension: size,
        child: TweenAnimationBuilder<double>(
          tween: Tween(begin: 0, end: v),
          duration: NectarTokens.medium * 2,
          curve: Curves.easeOutCubic,
          builder: (context, animated, _) => CustomPaint(
            painter: _RingPainter(animated, _color(c), c.border, size * 0.085),
            child: Center(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(
                    unavailable ? '—' : '${(animated * 100).round()}%',
                    style: theme.textTheme.headlineSmall?.copyWith(
                      fontSize: size * 0.2,
                    ),
                  ),
                  Text(label, style: theme.textTheme.labelMedium),
                  if (caption != null)
                    Padding(
                      padding: const EdgeInsets.only(top: 2),
                      child: Text(
                        caption!,
                        style: theme.textTheme.bodySmall?.copyWith(
                          fontSize: 11,
                        ),
                      ),
                    ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _RingPainter extends CustomPainter {
  _RingPainter(this.value, this.color, this.track, this.stroke);

  final double value;
  final Color color;
  final Color track;
  final double stroke;

  @override
  void paint(Canvas canvas, Size size) {
    final rect = (Offset.zero & size).deflate(stroke / 2);
    const start = math.pi * 0.75; // open at the bottom
    const sweep = math.pi * 1.5;
    final base = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = stroke
      ..strokeCap = StrokeCap.round;
    canvas.drawArc(rect, start, sweep, false, base..color = track);
    if (value > 0) {
      canvas.drawArc(rect, start, sweep * value, false, base..color = color);
    }
  }

  @override
  bool shouldRepaint(_RingPainter old) =>
      old.value != value || old.color != color || old.track != track;
}

/// Thin horizontal usage bar with a label row ("CPU  0.5 / 2 cores").
class UsageBar extends StatelessWidget {
  const UsageBar({
    super.key,
    required this.label,
    required this.value,
    this.trailing,
    this.unlimited = false,
  });

  final String label;
  final double value;
  final String? trailing;
  final bool unlimited;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final v = value.clamp(0.0, 1.0);
    final color = unlimited
        ? c.stopped
        : (v >= 0.9 ? c.failed : (v >= 0.75 ? c.warning : NectarTokens.amber));
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            Expanded(
              child: Text(
                label,
                style: Theme.of(context).textTheme.labelMedium,
              ),
            ),
            if (trailing != null)
              Text(trailing!, style: Theme.of(context).textTheme.bodySmall),
          ],
        ),
        const SizedBox(height: 6),
        ClipRRect(
          borderRadius: BorderRadius.circular(99),
          child: LinearProgressIndicator(
            value: unlimited ? 0 : v,
            minHeight: 6,
            backgroundColor: c.border,
            valueColor: AlwaysStoppedAnimation(color),
          ),
        ),
      ],
    );
  }
}

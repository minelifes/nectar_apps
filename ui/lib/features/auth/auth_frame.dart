import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../../theme/tokens.dart';
import '../../widgets/hex.dart';

/// Centered card on a faint honeycomb background, used by the sign-in pages.
class AuthFrame extends StatelessWidget {
  const AuthFrame({super.key, required this.title, this.subtitle, required this.children, this.footer});

  final String title;
  final String? subtitle;
  final List<Widget> children;
  final Widget? footer;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final theme = Theme.of(context);
    return Scaffold(
      body: Stack(
        children: [
          Positioned.fill(child: CustomPaint(painter: _HoneycombPainter(c.border.withValues(alpha: 0.5)))),
          Positioned.fill(
            child: DecoratedBox(
              decoration: BoxDecoration(
                gradient: RadialGradient(
                  radius: 0.9,
                  colors: [c.background.withValues(alpha: 0.2), c.background],
                ),
              ),
            ),
          ),
          SafeArea(
            child: Center(
              child: SingleChildScrollView(
                padding: const EdgeInsets.all(20),
                child: ConstrainedBox(
                  constraints: const BoxConstraints(maxWidth: 420),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      const Center(child: NectarMark(size: 56)),
                      const SizedBox(height: 18),
                      Text(title, textAlign: TextAlign.center, style: theme.textTheme.headlineSmall),
                      if (subtitle != null) ...[
                        const SizedBox(height: 6),
                        Text(subtitle!, textAlign: TextAlign.center, style: theme.textTheme.bodyMedium?.copyWith(color: c.muted)),
                      ],
                      const SizedBox(height: 24),
                      Card(
                        child: Padding(
                          padding: const EdgeInsets.all(24),
                          child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: children),
                        ),
                      ),
                      if (footer != null) ...[const SizedBox(height: 16), footer!],
                    ],
                  ),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _HoneycombPainter extends CustomPainter {
  _HoneycombPainter(this.color);

  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    const r = 34.0;
    final w = r * math.sqrt(3);
    final paint = Paint()
      ..color = color
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1;
    var row = 0;
    for (var y = -r; y < size.height + r; y += r * 1.5, row++) {
      final offset = row.isOdd ? w / 2 : 0.0;
      for (var x = -w + offset; x < size.width + w; x += w) {
        canvas.drawPath(hexagonPath(Rect.fromCenter(center: Offset(x, y), width: w, height: 2 * r)), paint);
      }
    }
  }

  @override
  bool shouldRepaint(_HoneycombPainter old) => old.color != color;
}

/// Inline error message inside a form.
class FormError extends StatelessWidget {
  const FormError(this.message, {super.key});

  final String message;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    return Container(
      margin: const EdgeInsets.only(bottom: 16),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: c.failed.withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(NectarTokens.radiusSmall),
        border: Border.all(color: c.failed.withValues(alpha: 0.35)),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(Icons.error_outline_rounded, size: 18, color: c.failed),
          const SizedBox(width: 10),
          Expanded(child: Text(message, style: TextStyle(color: c.text, fontSize: 13.5))),
        ],
      ),
    );
  }
}

/// Primary button that shows a loader while [busy].
class BusyButton extends StatelessWidget {
  const BusyButton({super.key, required this.label, required this.busy, required this.onPressed});

  final String label;
  final bool busy;
  final VoidCallback? onPressed;

  @override
  Widget build(BuildContext context) => FilledButton(
        onPressed: busy ? null : onPressed,
        child: busy
            ? const SizedBox.square(
                dimension: 18,
                child: CircularProgressIndicator(strokeWidth: 2.2, color: NectarTokens.onAmber),
              )
            : Text(label),
      );
}

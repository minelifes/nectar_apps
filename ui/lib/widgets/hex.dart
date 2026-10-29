import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../theme/tokens.dart';

/// Path of a pointy-top hexagon that fits [rect] (height-limited, centered).
Path hexagonPath(Rect rect, {double cornerRadius = 0}) {
  final r = math.min(rect.height / 2, rect.width / math.sqrt(3));
  final c = rect.center;
  final pts = [
    for (var i = 0; i < 6; i++)
      Offset(c.dx + r * math.cos((60 * i - 90) * math.pi / 180), c.dy + r * math.sin((60 * i - 90) * math.pi / 180)),
  ];
  final path = Path();
  if (cornerRadius <= 0) {
    path.addPolygon(pts, true);
    return path;
  }
  // rounded corners: cut each corner and join with a quadratic curve through the vertex
  for (var i = 0; i < 6; i++) {
    final prev = pts[(i + 5) % 6], cur = pts[i], next = pts[(i + 1) % 6];
    final a = cur + _towards(cur, prev, cornerRadius);
    final b = cur + _towards(cur, next, cornerRadius);
    if (i == 0) {
      path.moveTo(a.dx, a.dy);
    } else {
      path.lineTo(a.dx, a.dy);
    }
    path.quadraticBezierTo(cur.dx, cur.dy, b.dx, b.dy);
  }
  path.close();
  return path;
}

Offset _towards(Offset from, Offset to, double d) {
  final v = to - from;
  return v / v.distance * d;
}

/// A hexagon [ShapeBorder] for `Material`, `Ink` and `ShapeDecoration`.
class HexagonBorder extends OutlinedBorder {
  const HexagonBorder({super.side, this.cornerRadius = 0});

  final double cornerRadius;

  @override
  Path getInnerPath(Rect rect, {TextDirection? textDirection}) =>
      hexagonPath(rect.deflate(side.strokeInset), cornerRadius: cornerRadius);

  @override
  Path getOuterPath(Rect rect, {TextDirection? textDirection}) => hexagonPath(rect, cornerRadius: cornerRadius);

  @override
  void paint(Canvas canvas, Rect rect, {TextDirection? textDirection}) {
    if (side.style == BorderStyle.none) return;
    canvas.drawPath(hexagonPath(rect.deflate(side.strokeInset / 2), cornerRadius: cornerRadius), side.toPaint());
  }

  @override
  ShapeBorder scale(double t) => HexagonBorder(side: side.scale(t), cornerRadius: cornerRadius * t);

  @override
  HexagonBorder copyWith({BorderSide? side, double? cornerRadius}) =>
      HexagonBorder(side: side ?? this.side, cornerRadius: cornerRadius ?? this.cornerRadius);

  @override
  bool operator ==(Object other) => other is HexagonBorder && other.side == side && other.cornerRadius == cornerRadius;

  @override
  int get hashCode => Object.hash(side, cornerRadius);
}

/// A filled hexagon badge with a centered child (runtime glyph, initials, icon).
class HexBadge extends StatelessWidget {
  const HexBadge({super.key, required this.child, this.size = 40, this.color, this.gradient});

  final Widget child;
  final double size;
  final Color? color;
  final Gradient? gradient;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: size * math.sqrt(3) / 2,
      height: size,
      child: DecoratedBox(
        decoration: ShapeDecoration(
          shape: HexagonBorder(cornerRadius: size * 0.08),
          color: gradient == null ? (color ?? NectarTokens.amber) : null,
          gradient: gradient,
        ),
        child: Center(child: child),
      ),
    );
  }
}

/// The Nectar Apps mark: three honeycomb cells (the app icon without its background).
class NectarMark extends StatelessWidget {
  const NectarMark({super.key, this.size = 36});

  final double size;

  @override
  Widget build(BuildContext context) =>
      SizedBox.square(dimension: size, child: CustomPaint(painter: _MarkPainter()));
}

class _MarkPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    // cell radius so that two cells side by side + one on top fit the square
    final r = size.width / (2 * math.sqrt(3) + 0.12);
    final w = r * math.sqrt(3);
    final gap = r * 0.05;
    final cy = size.height / 2 + r * 0.72;
    final cells = <(Offset, List<Color>)>[
      (Offset(size.width / 2 - w / 2 - gap, cy), const [Color(0xFFFFC94A), Color(0xFFF08A00)]),
      (Offset(size.width / 2 + w / 2 + gap, cy), const [Color(0xFFF5A524), Color(0xFFD96F00)]),
      (Offset(size.width / 2, cy - 1.5 * r - gap * 1.7), const [Color(0xFFFFE08A), Color(0xFFFFB940)]),
    ];
    for (final (center, colors) in cells) {
      final rect = Rect.fromCenter(center: center, width: w, height: 2 * r);
      final paint = Paint()
        ..shader = LinearGradient(begin: Alignment.topLeft, end: Alignment.bottomRight, colors: colors).createShader(rect);
      canvas.drawPath(hexagonPath(rect, cornerRadius: r * 0.06), paint);
    }
    // chevron in the top cell ("run")
    final top = cells[2].$1;
    final s = r * 0.32;
    final chevron = Path()
      ..moveTo(top.dx - s * 0.6, top.dy - s)
      ..lineTo(top.dx + s * 0.6, top.dy)
      ..lineTo(top.dx - s * 0.6, top.dy + s);
    canvas.drawPath(
      chevron,
      Paint()
        ..color = const Color(0xFF3A2400)
        ..style = PaintingStyle.stroke
        ..strokeWidth = r * 0.2
        ..strokeCap = StrokeCap.round
        ..strokeJoin = StrokeJoin.round,
    );
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}

/// Loader: a hexagon that fills with nectar, repeatedly.
class HexLoader extends StatefulWidget {
  const HexLoader({super.key, this.size = 40});

  final double size;

  @override
  State<HexLoader> createState() => _HexLoaderState();
}

class _HexLoaderState extends State<HexLoader> with SingleTickerProviderStateMixin {
  late final AnimationController _c = AnimationController(vsync: this, duration: const Duration(milliseconds: 1400))
    ..repeat();

  @override
  void dispose() {
    _c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final colors = NectarColors.of(context);
    return Semantics(
      label: 'Loading',
      child: SizedBox(
        width: widget.size * math.sqrt(3) / 2,
        height: widget.size,
        child: AnimatedBuilder(
          animation: _c,
          builder: (context, _) => CustomPaint(painter: _FillPainter(_c.value, colors.border)),
        ),
      ),
    );
  }
}

class _FillPainter extends CustomPainter {
  _FillPainter(this.t, this.track);

  final double t;
  final Color track;

  @override
  void paint(Canvas canvas, Size size) {
    final rect = Offset.zero & size;
    final hex = hexagonPath(rect, cornerRadius: size.height * 0.08);
    canvas.drawPath(hex, Paint()..color = track);
    final fill = Curves.easeInOut.transform((t / 0.6).clamp(0.0, 1.0));
    final fade = t < 0.6 ? 1.0 : 1 - (t - 0.6) / 0.4;
    canvas.save();
    canvas.clipPath(hex);
    canvas.drawRect(
      Rect.fromLTRB(0, size.height * (1 - fill), size.width, size.height),
      Paint()..color = NectarTokens.amber.withValues(alpha: fade),
    );
    canvas.restore();
  }

  @override
  bool shouldRepaint(_FillPainter old) => old.t != t || old.track != track;
}

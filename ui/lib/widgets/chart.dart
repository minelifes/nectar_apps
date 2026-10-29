import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../theme/tokens.dart';

/// One line of a [TimeSeriesChart].
class ChartSeries {
  const ChartSeries({required this.label, required this.color, required this.points, this.fill = false, this.dashed = false});

  final String label;
  final Color color;
  final List<(DateTime, double)> points;

  /// Gradient area under the line.
  final bool fill;

  /// Dashed reference line (e.g. requested resources).
  final bool dashed;
}

/// Small time-series line chart with a hover/touch crosshair. No dependencies: drawn with a painter.
class TimeSeriesChart extends StatefulWidget {
  const TimeSeriesChart({
    super.key,
    required this.series,
    required this.formatY,
    this.height = 150,
    this.yMax,
    this.from,
    this.to,
  });

  final List<ChartSeries> series;
  final String Function(double) formatY;
  final double height;

  /// Minimum top of the y axis (e.g. a capacity); the axis grows when data is higher.
  final double? yMax;
  final DateTime? from;
  final DateTime? to;

  @override
  State<TimeSeriesChart> createState() => _TimeSeriesChartState();
}

class _TimeSeriesChartState extends State<TimeSeriesChart> {
  double? _hoverX;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final empty = widget.series.every((s) => s.points.isEmpty);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Wrap(
          spacing: 14,
          runSpacing: 4,
          children: [
            for (final s in widget.series)
              Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Container(width: 10, height: 3, decoration: BoxDecoration(color: s.color, borderRadius: BorderRadius.circular(2))),
                  const SizedBox(width: 6),
                  Text(
                    s.points.isEmpty ? s.label : '${s.label} ${widget.formatY(s.points.last.$2)}',
                    style: TextStyle(fontSize: 12, color: c.muted),
                  ),
                ],
              ),
          ],
        ),
        const SizedBox(height: 8),
        SizedBox(
          height: widget.height,
          child: empty
              ? Center(child: Text('No data yet: history fills in every 15 seconds.', style: TextStyle(color: c.muted, fontSize: 12.5)))
              : LayoutBuilder(
                  builder: (context, box) => MouseRegion(
                    onHover: (e) => setState(() => _hoverX = e.localPosition.dx),
                    onExit: (_) => setState(() => _hoverX = null),
                    child: GestureDetector(
                      onPanDown: (d) => setState(() => _hoverX = d.localPosition.dx),
                      onPanUpdate: (d) => setState(() => _hoverX = d.localPosition.dx),
                      onPanEnd: (_) => setState(() => _hoverX = null),
                      onPanCancel: () => setState(() => _hoverX = null),
                      child: CustomPaint(
                        size: Size(box.maxWidth, widget.height),
                        painter: _ChartPainter(
                          series: widget.series,
                          formatY: widget.formatY,
                          yMax: widget.yMax,
                          from: widget.from,
                          to: widget.to,
                          hoverX: _hoverX,
                          grid: c.border,
                          text: c.muted,
                          tooltipBg: c.elevated,
                          tooltipText: c.text,
                        ),
                      ),
                    ),
                  ),
                ),
        ),
      ],
    );
  }
}

class _ChartPainter extends CustomPainter {
  _ChartPainter({
    required this.series,
    required this.formatY,
    required this.yMax,
    required this.from,
    required this.to,
    required this.hoverX,
    required this.grid,
    required this.text,
    required this.tooltipBg,
    required this.tooltipText,
  });

  final List<ChartSeries> series;
  final String Function(double) formatY;
  final double? yMax;
  final DateTime? from;
  final DateTime? to;
  final double? hoverX;
  final Color grid;
  final Color text;
  final Color tooltipBg;
  final Color tooltipText;

  static const _left = 56.0;
  static const _bottom = 18.0;

  @override
  void paint(Canvas canvas, Size size) {
    final all = [for (final s in series) ...s.points];
    if (all.isEmpty) return;
    final t0 = (from ?? all.map((p) => p.$1).reduce((a, b) => a.isBefore(b) ? a : b)).millisecondsSinceEpoch.toDouble();
    var t1 = (to ?? all.map((p) => p.$1).reduce((a, b) => a.isAfter(b) ? a : b)).millisecondsSinceEpoch.toDouble();
    if (t1 <= t0) t1 = t0 + 60000;
    var top = all.map((p) => p.$2).fold<double>(0, math.max) * 1.15;
    if (yMax != null) top = math.max(top, yMax!);
    if (top <= 0) top = 1;

    final plot = Rect.fromLTRB(_left, 4, size.width - 4, size.height - _bottom);
    double x(DateTime t) => plot.left + (t.millisecondsSinceEpoch - t0) / (t1 - t0) * plot.width;
    double y(double v) => plot.bottom - (v / top).clamp(0, 1) * plot.height;

    // grid + y labels
    final gridPaint = Paint()
      ..color = grid
      ..strokeWidth = 1;
    for (var i = 0; i <= 3; i++) {
      final v = top * i / 3;
      final gy = y(v);
      canvas.drawLine(Offset(plot.left, gy), Offset(plot.right, gy), gridPaint..color = grid.withValues(alpha: i == 0 ? 1 : 0.5));
      _text(canvas, formatY(v), Offset(plot.left - 6, gy), align: TextAlign.right, anchorRight: true);
    }
    // x labels: start, middle, end
    final span = Duration(milliseconds: (t1 - t0).round());
    for (final f in [0.0, 0.5, 1.0]) {
      final t = DateTime.fromMillisecondsSinceEpoch((t0 + (t1 - t0) * f).round());
      final label = _timeLabel(t, span);
      final px = plot.left + plot.width * f;
      _text(canvas, label, Offset(px, plot.bottom + 3), center: f == 0.5, anchorRight: f == 1.0, top: true);
    }

    // lines
    for (final s in series) {
      if (s.points.isEmpty) continue;
      final path = Path();
      for (var i = 0; i < s.points.length; i++) {
        final p = Offset(x(s.points[i].$1), y(s.points[i].$2));
        if (i == 0) {
          path.moveTo(p.dx, p.dy);
        } else {
          path.lineTo(p.dx, p.dy);
        }
      }
      if (s.fill && s.points.length > 1) {
        final area = Path.from(path)
          ..lineTo(x(s.points.last.$1), plot.bottom)
          ..lineTo(x(s.points.first.$1), plot.bottom)
          ..close();
        canvas.drawPath(
          area,
          Paint()
            ..shader = LinearGradient(
              begin: Alignment.topCenter,
              end: Alignment.bottomCenter,
              colors: [s.color.withValues(alpha: 0.28), s.color.withValues(alpha: 0.02)],
            ).createShader(plot),
        );
      }
      final paint = Paint()
        ..color = s.color
        ..strokeWidth = s.dashed ? 1.2 : 1.8
        ..style = PaintingStyle.stroke
        ..strokeJoin = StrokeJoin.round;
      if (s.dashed) {
        _dashed(canvas, path, paint);
      } else {
        canvas.drawPath(path, paint);
      }
    }

    // crosshair
    final hx = hoverX;
    if (hx == null || hx < plot.left || hx > plot.right) return;
    final tHover = t0 + (hx - plot.left) / plot.width * (t1 - t0);
    canvas.drawLine(Offset(hx, plot.top), Offset(hx, plot.bottom), Paint()..color = text.withValues(alpha: 0.5));
    final rows = <String>[];
    DateTime? at;
    for (final s in series) {
      if (s.points.isEmpty) continue;
      var best = s.points.first;
      for (final p in s.points) {
        if ((p.$1.millisecondsSinceEpoch - tHover).abs() < (best.$1.millisecondsSinceEpoch - tHover).abs()) best = p;
      }
      at ??= best.$1;
      canvas.drawCircle(Offset(x(best.$1), y(best.$2)), 3.5, Paint()..color = s.color);
      rows.add('${s.label}: ${formatY(best.$2)}');
    }
    if (at == null) return;
    rows.insert(0, _timeLabel(at, const Duration(hours: 1), seconds: true));
    final tp = TextPainter(
      text: TextSpan(text: rows.join('\n'), style: TextStyle(fontSize: 11.5, color: tooltipText, height: 1.4)),
      textDirection: TextDirection.ltr,
    )..layout();
    var bx = hx + 10;
    if (bx + tp.width + 16 > size.width) bx = hx - tp.width - 26;
    final box = RRect.fromRectAndRadius(Rect.fromLTWH(bx, plot.top + 4, tp.width + 16, tp.height + 12), const Radius.circular(8));
    canvas.drawRRect(box, Paint()..color = tooltipBg);
    canvas.drawRRect(box, Paint()
      ..color = grid
      ..style = PaintingStyle.stroke);
    tp.paint(canvas, Offset(bx + 8, plot.top + 10));
  }

  String _timeLabel(DateTime t, Duration span, {bool seconds = false}) {
    String two(int n) => n.toString().padLeft(2, '0');
    final hm = '${two(t.hour)}:${two(t.minute)}${seconds ? ':${two(t.second)}' : ''}';
    return span > const Duration(days: 2) ? '${two(t.month)}-${two(t.day)} $hm' : hm;
  }

  void _text(Canvas canvas, String s, Offset at, {TextAlign align = TextAlign.left, bool anchorRight = false, bool center = false, bool top = false}) {
    final tp = TextPainter(
      text: TextSpan(text: s, style: TextStyle(fontSize: 10.5, color: text)),
      textDirection: TextDirection.ltr,
      textAlign: align,
    )..layout();
    var dx = at.dx;
    if (anchorRight) dx -= tp.width;
    if (center) dx -= tp.width / 2;
    final dy = top ? at.dy : at.dy - tp.height / 2;
    tp.paint(canvas, Offset(dx, dy));
  }

  void _dashed(Canvas canvas, Path path, Paint paint) {
    for (final m in path.computeMetrics()) {
      var d = 0.0;
      while (d < m.length) {
        canvas.drawPath(m.extractPath(d, math.min(d + 5, m.length)), paint);
        d += 9;
      }
    }
  }

  @override
  bool shouldRepaint(_ChartPainter old) =>
      old.series != series || old.hoverX != hoverX || old.yMax != yMax || old.grid != grid || old.from != from || old.to != to;
}

/// Segmented range picker for usage charts.
class RangePicker extends StatelessWidget {
  const RangePicker({super.key, required this.value, required this.options, required this.onChanged});

  final String value;
  final List<String> options;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) => SegmentedButton<String>(
        segments: [for (final o in options) ButtonSegment(value: o, label: Text(o))],
        selected: {value},
        showSelectedIcon: false,
        style: const ButtonStyle(visualDensity: VisualDensity.compact),
        onSelectionChanged: (s) => onChanged(s.first),
      );
}

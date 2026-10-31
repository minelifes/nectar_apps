import 'package:flutter/material.dart';

import '../api/models.dart';
import '../core/format.dart';
import '../theme/tokens.dart';
import 'hex.dart';
import 'status.dart';

/// App tile: runtime hex badge, name, runtime + version, status pill, replicas and resources.
class AppTile extends StatefulWidget {
  const AppTile({super.key, required this.app, this.onTap, this.width = 280});

  final App app;
  final VoidCallback? onTap;
  final double width;

  @override
  State<AppTile> createState() => _AppTileState();
}

class _AppTileState extends State<AppTile> {
  bool _hover = false;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final app = widget.app;
    final rt = runtimeInfo(app.runtime);
    final tone = toneColor(context, toneFor(app.status));
    final theme = Theme.of(context);
    final version = app.runtime == 'image'
        ? _shortImage(app.image)
        : app.runtimeVersion;
    final tile = AnimatedContainer(
      duration: NectarTokens.fast,
      width: widget.width,
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: _hover ? c.elevated : c.surface,
        borderRadius: BorderRadius.circular(NectarTokens.radius),
        border: Border.all(
          color: _hover ? tone.withValues(alpha: 0.45) : c.border,
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              HexBadge(
                size: 38,
                gradient: LinearGradient(
                  begin: Alignment.topLeft,
                  end: Alignment.bottomRight,
                  colors: [
                    tone.withValues(alpha: 0.28),
                    tone.withValues(alpha: 0.12),
                  ],
                ),
                child: Icon(rt.icon, size: 17, color: tone),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      app.name,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: theme.textTheme.titleMedium,
                    ),
                    Text(
                      [rt.label, if (version.isNotEmpty) version].join(' · '),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: theme.textTheme.bodySmall,
                    ),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 14),
          Row(
            children: [
              Tooltip(
                message: app.statusMessage,
                child: StatusPill(status: app.status),
              ),
              const Spacer(),
              if (app.kind == 'web' || app.kind == 'worker')
                Text(
                  '${app.replicas}× · ${formatCpu(app.cpuMillicores)} · ${formatMb(app.memoryMb)}',
                  style: theme.textTheme.bodySmall,
                )
              else
                Text(app.kind, style: theme.textTheme.bodySmall),
            ],
          ),
        ],
      ),
    );
    return MouseRegion(
      cursor: widget.onTap == null
          ? MouseCursor.defer
          : SystemMouseCursors.click,
      onEnter: (_) => setState(() => _hover = true),
      onExit: (_) => setState(() => _hover = false),
      child: GestureDetector(onTap: widget.onTap, child: tile),
    );
  }

  static String _shortImage(String image) {
    final last = image.split('/').last;
    return last.length > 28 ? '${last.substring(0, 27)}…' : last;
  }
}

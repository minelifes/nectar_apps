import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../api/models.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/gauge.dart';

/// Every app and database volume on the server with its usage (known while a pod mounts it).
class VolumesCard extends ConsumerWidget {
  const VolumesCard({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final vols = ref.watch(serverVolumesProvider);
    final c = NectarColors.of(context);
    return NCard(
      title: s.serverVolumes,
      trailing: IconButton(
        tooltip: s.actionRefresh,
        icon: const Icon(Icons.refresh_rounded, size: 18),
        onPressed: () => ref.invalidate(serverVolumesProvider),
      ),
      child: AsyncBlock(
        value: vols,
        loadingHeight: 80,
        onRetry: () => ref.invalidate(serverVolumesProvider),
        data: (list) {
          if (list.isEmpty) {
            return Text(
              s.serverNoVolumesYetAppsGet,
              style: TextStyle(color: c.muted),
            );
          }
          final sorted = [...list]
            ..sort((a, b) => _share(b).compareTo(_share(a)));
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              for (final v in sorted) ...[
                InkWell(
                  onTap: v.appId > 0
                      ? () => context.go('/apps/${v.appId}')
                      : null,
                  child: Padding(
                    padding: const EdgeInsets.symmetric(vertical: 6),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(v.label, style: const TextStyle(fontSize: 13.5)),
                        const SizedBox(height: 4),
                        if (v.usedBytes >= 0 && v.requestedBytes > 0)
                          UsageBar(
                            label: v.storageClass.isEmpty
                                ? 'Used'
                                : v.storageClass,
                            value: v.usedBytes / v.requestedBytes,
                            trailing:
                                '${formatBytes(v.usedBytes)} of ${formatBytes(v.requestedBytes)}'
                                '${v.availableBytes >= 0 ? ' · ${formatBytes(v.availableBytes)} free on ${v.node.isEmpty ? 'the disk' : v.node}' : ''}',
                          )
                        else
                          Text(
                            s.serverUsageShowsWhileTheApp(
                              formatBytes(v.requestedBytes),
                            ),
                            style: TextStyle(color: c.muted, fontSize: 12),
                          ),
                      ],
                    ),
                  ),
                ),
                const Divider(height: 1),
              ],
              const SizedBox(height: 8),
              Text(
                s.serverOnLocalPathStorageThe,
                style: TextStyle(color: c.muted, fontSize: 12),
              ),
            ],
          );
        },
      ),
    );
  }

  static double _share(VolumeUse v) => v.usedBytes < 0 || v.requestedBytes <= 0
      ? -1
      : v.usedBytes / v.requestedBytes;
}

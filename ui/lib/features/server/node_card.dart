import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/session.dart';
import '../../data/providers.dart';

import '../../api/models.dart';
import '../../core/format.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/gauge.dart';
import '../../widgets/status.dart';

/// One node: CPU and memory gauges plus its facts.
class NodeCard extends ConsumerWidget {
  const NodeCard({super.key, required this.node, this.multiNode = false});

  final NodeInfo node;
  final bool multiNode;

  Future<void> _action(BuildContext context, WidgetRef ref, String action) async {
    final api = ref.read(apiProvider);
    try {
      switch (action) {
        case 'drain':
          if (!await confirmAction(context, 'Drain ${node.name}?',
              'No new pods start there, and its pods move to the other nodes (DaemonSet pods stay). Apps with one replica are briefly unavailable.', 'Drain')) {
            return;
          }
          Map res;
          try {
            res = await api.post('/server/nodes/${node.name}/drain') as Map;
          } on ApiException catch (e) {
            // 409: draining would take things offline (edge node, or no node left for the pods)
            if (e.status != 409 || !context.mounted) rethrow;
            if (!await confirmAction(context, 'Drain ${node.name} anyway?', e.message, 'Drain anyway', danger: true)) return;
            res = await api.post('/server/nodes/${node.name}/drain?force=true') as Map;
          }
          final failed = (res['failed'] as List?) ?? const [];
          if (context.mounted) {
            showSnack(context, failed.isEmpty ? 'Moved ${(res['evicted'] as List).length} pods away from ${node.name}' : 'Some pods could not move: ${failed.join('; ')}',
                error: failed.isNotEmpty);
          }
        case 'remove':
          if (!await confirmAction(context, 'Remove ${node.name} from the cluster?',
              'The node object is deleted. On the machine, run /usr/local/bin/k3s-agent-uninstall.sh to remove k3s.', 'Remove', danger: true)) {
            return;
          }
          await api.delete('/server/nodes/${node.name}');
        default:
          await api.post('/server/nodes/${node.name}/$action');
      }
      ref.invalidate(nodesProvider);
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final canManage = ref.watch(sessionProvider.select((s) => s.can('server.manage')));
    final controlPlane = node.roles.contains('control-plane') || node.roles.contains('master');
    final c = NectarColors.of(context);
    final noMetrics = node.usage == null;
    final usage = node.usage;
    final gauges = Wrap(
      spacing: 20,
      runSpacing: 12,
      children: [
        RingGauge(
          label: 'CPU',
          value: node.cpuRatio ?? 0,
          unavailable: noMetrics,
          caption: usage == null ? 'of ${formatCpu(node.allocatable.cpuMillicores)}' : '${formatCpu(usage.cpuMillicores)} / ${formatCpu(node.allocatable.cpuMillicores)}',
        ),
        RingGauge(
          label: 'Memory',
          value: node.memoryRatio ?? 0,
          unavailable: noMetrics,
          caption: usage == null ? 'of ${formatBytes(node.allocatable.memoryBytes)}' : '${formatBytes(usage.memoryBytes)} / ${formatBytes(node.allocatable.memoryBytes)}',
        ),
      ],
    );
    final facts = Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _Fact('Node', node.name, mono: true),
        if (node.roles.isNotEmpty || node.edge) _Fact('Roles', [...node.roles, if (node.edge) 'edge (gateway)'].join(', ')),
        if (node.unschedulable) _Fact('Scheduling', 'cordoned: no new pods'),
        if (node.internalIp.isNotEmpty) _Fact('IP', node.externalIp.isNotEmpty ? '${node.externalIp} · ${node.internalIp}' : node.internalIp, mono: true),
        if (node.os.isNotEmpty) _Fact('OS', '${node.os}${node.arch.isEmpty ? '' : ' (${node.arch})'}'),
        if (node.kubelet.isNotEmpty) _Fact('Kubernetes', node.kubelet, mono: true),
        if (node.allocatable.storageBytes > 0) _Fact('Disk', formatBytes(node.allocatable.storageBytes)),
        if (noMetrics)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text('Live usage needs metrics-server (k3s includes it).', style: TextStyle(color: c.muted, fontSize: 12)),
          ),
      ],
    );
    return NCard(
      title: multiNode ? node.name : 'Server',
      trailing: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          StatusPill(status: node.ready ? (node.unschedulable ? 'warning' : 'running') : 'failed', label: node.ready ? (node.unschedulable ? 'Cordoned' : 'Ready') : 'Not ready'),
          if (canManage && multiNode)
            PopupMenuButton<String>(
              tooltip: 'Node actions',
              onSelected: (a) => _action(context, ref, a),
              itemBuilder: (_) => [
                if (node.unschedulable)
                  const PopupMenuItem(value: 'uncordon', child: Text('Allow new pods (uncordon)'))
                else
                  const PopupMenuItem(value: 'cordon', child: Text('No new pods (cordon)')),
                const PopupMenuItem(value: 'drain', child: Text('Drain (move pods away)')),
                if (!controlPlane && !node.edge && node.unschedulable) const PopupMenuItem(value: 'remove', child: Text('Remove from the cluster')),
              ],
            ),
        ],
      ),
      child: LayoutBuilder(
        builder: (context, box) => box.maxWidth >= 560
            ? Row(crossAxisAlignment: CrossAxisAlignment.start, children: [gauges, const SizedBox(width: 28), Expanded(child: facts)])
            : Column(crossAxisAlignment: CrossAxisAlignment.start, children: [gauges, const SizedBox(height: 18), facts]),
      ),
    );
  }
}

class _Fact extends StatelessWidget {
  const _Fact(this.label, this.value, {this.mono = false});

  final String label;
  final String value;
  final bool mono;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(width: 96, child: Text(label, style: TextStyle(color: c.muted, fontSize: 13))),
          Expanded(
            child: SelectableText(value, style: mono ? monoStyle(context, size: 12.5) : const TextStyle(fontSize: 13.5)),
          ),
        ],
      ),
    );
  }
}

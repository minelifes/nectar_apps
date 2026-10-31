import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/session.dart';
import '../../data/providers.dart';

import '../../api/models.dart';
import '../../core/format.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/gauge.dart';
import '../../widgets/status.dart';

/// GET /server/edge: the primary edge node and the gateway replicas with their sync state.
final edgeStatusProvider = FutureProvider.autoDispose<Map<String, dynamic>>((ref) async {
  final v = await ref.watch(apiProvider).get('/server/edge');
  return v is Map ? Map<String, dynamic>.from(v) : const {};
});

/// One node: CPU and memory gauges plus its facts.
class NodeCard extends ConsumerWidget {
  const NodeCard({super.key, required this.node, this.multiNode = false});

  final NodeInfo node;
  final bool multiNode;

  Future<void> _action(BuildContext context, WidgetRef ref, String action) async {
    final s = S.of(context);
    final api = ref.read(apiProvider);
    try {
      switch (action) {
        case 'drain':
          if (!await confirmAction(context, s.serverDrain(node.name),
              s.serverNoNewPodsStartThere, s.serverDrain2)) {
            return;
          }
          Map res;
          try {
            res = await api.post('/server/nodes/${node.name}/drain') as Map;
          } on ApiException catch (e) {
            // 409: draining would take things offline (edge node, or no node left for the pods)
            if (e.status != 409 || !context.mounted) rethrow;
            if (!await confirmAction(context, s.serverDrainAnyway(node.name), e.message, s.serverDrainAnyway2, danger: true)) return;
            res = await api.post('/server/nodes/${node.name}/drain?force=true') as Map;
          }
          final failed = (res['failed'] as List?) ?? const [];
          if (context.mounted) {
            showSnack(context, failed.isEmpty ? 'Moved ${(res['evicted'] as List).length} pods away from ${node.name}' : 'Some pods could not move: ${failed.join('; ')}',
                error: failed.isNotEmpty);
          }
        case 'edge-on':
          if (!await confirmAction(context, s.serverMakeAnEdgeNode(node.name),
              s.serverANectarGatewayReplicaStarts,
              s.serverMakeEdgeNode)) {
            return;
          }
          await api.post('/server/nodes/${node.name}/edge-on');
        case 'edge-off':
          if (!await confirmAction(context, s.serverStopServingTrafficOn(node.name),
              s.serverItsGatewayReplicaStopsAnd,
              s.serverStop, danger: true)) {
            return;
          }
          await api.post('/server/nodes/${node.name}/edge-off');
        case 'remove':
          if (!await confirmAction(context, s.serverRemoveFromTheCluster(node.name),
              s.serverTheNodeObjectIsDeleted, s.backupRemove2, danger: true)) {
            return;
          }
          await api.delete('/server/nodes/${node.name}');
        default:
          await api.post('/server/nodes/${node.name}/$action');
      }
      ref.invalidate(nodesProvider);
      ref.invalidate(edgeStatusProvider);
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = S.of(context);
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
          label: l.serverCpu,
          value: node.cpuRatio ?? 0,
          unavailable: noMetrics,
          caption: usage == null ? 'of ${formatCpu(node.allocatable.cpuMillicores)}' : '${formatCpu(usage.cpuMillicores)} / ${formatCpu(node.allocatable.cpuMillicores)}',
        ),
        RingGauge(
          label: l.serverMemory,
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
        if (node.roles.isNotEmpty || node.edge || node.edgeReplica)
          _Fact('Roles', [...node.roles, if (node.edge) 'edge (gateway)', if (node.edgeReplica) 'edge (gateway replica)'].join(', ')),
        if (node.edgeReplica) _Fact('Gateway', _replicaState(ref.watch(edgeStatusProvider).value)),
        if (node.unschedulable) _Fact('Scheduling', 'cordoned: no new pods'),
        if (node.internalIp.isNotEmpty) _Fact('IP', node.externalIp.isNotEmpty ? '${node.externalIp} · ${node.internalIp}' : node.internalIp, mono: true),
        if (node.os.isNotEmpty) _Fact('OS', '${node.os}${node.arch.isEmpty ? '' : ' (${node.arch})'}'),
        if (node.kubelet.isNotEmpty) _Fact('Kubernetes', node.kubelet, mono: true),
        if (node.allocatable.storageBytes > 0) _Fact('Disk', formatBytes(node.allocatable.storageBytes)),
        if (noMetrics)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text(l.serverLiveUsageNeedsMetricsServer, style: TextStyle(color: c.muted, fontSize: 12)),
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
              tooltip: l.serverNodeActions,
              onSelected: (a) => _action(context, ref, a),
              itemBuilder: (_) => [
                if (node.unschedulable)
                  PopupMenuItem(value: 'uncordon', child: Text(l.serverAllowNewPodsUncordon))
                else
                  PopupMenuItem(value: 'cordon', child: Text(l.serverNoNewPodsCordon)),
                PopupMenuItem(value: 'drain', child: Text(l.serverDrainMovePodsAway)),
                if (!node.edge)
                  node.edgeReplica
                      ? PopupMenuItem(value: 'edge-off', child: Text(l.serverStopServingTrafficEdge))
                      : PopupMenuItem(value: 'edge-on', child: Text(l.serverServeTrafficTooEdgeNode)),
                if (!controlPlane && !node.edge && node.unschedulable) PopupMenuItem(value: 'remove', child: Text(l.serverRemoveFromTheCluster2)),
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

  String _replicaState(Map<String, dynamic>? status) {
    if (status == null) return 'replica';
    final r = ((status['replicas'] as List?) ?? const []).whereType<Map>().where((r) => r['node'] == node.name).firstOrNull;
    if (r == null) return 'replica';
    if (r['synced'] == true) return 'replica · in sync with ${status['primary'] ?? 'the main gateway'}';
    final err = '${r['error'] ?? ''}';
    if (err.isNotEmpty) return 'replica · not in sync: $err';
    return r['last_seen'] == null ? 'replica · starting (no sync yet)' : 'replica · last sync ${r['last_seen']}';
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

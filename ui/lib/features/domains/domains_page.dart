import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/status.dart';
import '../apps/domain_dialog.dart';

/// Every domain across projects: where it points, its HTTPS and DNS state, and how to set up DNS.
class DomainsPage extends ConsumerStatefulWidget {
  const DomainsPage({super.key});

  @override
  ConsumerState<DomainsPage> createState() => _DomainsPageState();
}

class _DomainsPageState extends ConsumerState<DomainsPage> {
  String _query = '';
  String _filter = 'all'; // all | problems | custom

  @override
  Widget build(BuildContext context) {
    final overview = ref.watch(domainsOverviewProvider);
    return PageScaffold(
      title: 'Domains',
      subtitle: 'Every domain of every app you can see, with its HTTPS and DNS state.',
      onRefresh: () async => ref.invalidate(domainsOverviewProvider),
      children: [
        AsyncBlock(
          value: overview,
          onRetry: () => ref.invalidate(domainsOverviewProvider),
          data: (o) => Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              _DnsSetup(overview: o),
              const SizedBox(height: NectarTokens.gap),
              _list(context, o),
            ],
          ),
        ),
      ],
    );
  }

  Widget _list(BuildContext context, DomainsOverview o) {
    final c = NectarColors.of(context);
    final q = _query.toLowerCase();
    final rows = o.domains.where((r) {
      final d = r.domain;
      if (q.isNotEmpty && !('${d.host}${d.pathPrefix} ${r.appName} ${r.projectName}'.toLowerCase().contains(q))) return false;
      return switch (_filter) {
        'problems' => d.status == 'error' || d.dnsState == 'error' || d.dnsState == 'conflict',
        'custom' => !d.isDefault,
        _ => true,
      };
    }).toList();
    return NCard(
      title: 'All domains',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Wrap(
            spacing: 10,
            runSpacing: 10,
            crossAxisAlignment: WrapCrossAlignment.center,
            children: [
              SizedBox(
                width: 260,
                child: TextField(
                  decoration: const InputDecoration(prefixIcon: Icon(Icons.search_rounded, size: 18), hintText: 'Search domains, apps, projects', isDense: true),
                  onChanged: (v) => setState(() => _query = v.trim()),
                ),
              ),
              SegmentedButton<String>(
                segments: const [
                  ButtonSegment(value: 'all', label: Text('All')),
                  ButtonSegment(value: 'custom', label: Text('Custom')),
                  ButtonSegment(value: 'problems', label: Text('Problems')),
                ],
                selected: {_filter},
                onSelectionChanged: (s) => setState(() => _filter = s.first),
                showSelectedIcon: false,
              ),
            ],
          ),
          const SizedBox(height: 12),
          if (o.domains.isEmpty)
            const EmptyState(icon: Icons.language_rounded, title: 'No domains yet', message: 'Web apps get a domain under the base domain when they are created; add your own on the app page.')
          else if (rows.isEmpty)
            Padding(padding: const EdgeInsets.all(12), child: Text('Nothing matches.', style: TextStyle(color: c.muted)))
          else
            for (final r in rows) ...[const Divider(height: 1), _DomainTile(row: r, overview: o)],
        ],
      ),
    );
  }
}

/// DNS instructions and the automatic-records switch.
class _DnsSetup extends ConsumerStatefulWidget {
  const _DnsSetup({required this.overview});

  final DomainsOverview overview;

  @override
  ConsumerState<_DnsSetup> createState() => _DnsSetupState();
}

class _DnsSetupState extends ConsumerState<_DnsSetup> {
  bool _busy = false;

  Future<void> _toggleAuto(bool on) async {
    setState(() => _busy = true);
    try {
      final api = ref.read(apiProvider);
      final cur = await api.get('/settings');
      final res = await api.put('/settings', data: {
        'base_domain': (cur as Map)['base_domain'] ?? '',
        'public_ips': cur['public_ips'] ?? const [],
        'dns_auto': on,
      });
      ref.invalidate(domainsOverviewProvider);
      final dns = res is Map ? res['dns'] : null;
      if (!mounted) return;
      if (dns is Map && dns['conflict'] != null) {
        showSnack(context, '${dns['conflict']}', error: true);
      } else {
        showSnack(context, on ? 'New domains get their DNS records automatically' : 'Automatic DNS records off');
      }
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final o = widget.overview;
    final c = NectarColors.of(context);
    final canManage = ref.watch(sessionProvider).can('settings.manage');
    final ips = o.serverIps;
    final v4 = ips.where((ip) => !ip.contains(':')).toList();
    final v6 = ips.where((ip) => ip.contains(':')).toList();
    final records = <(String, String, String)>[
      if (o.baseDomain.isNotEmpty) ...[
        for (final ip in v4) ('*.${o.baseDomain}', 'A', ip),
        for (final ip in v6) ('*.${o.baseDomain}', 'AAAA', ip),
      ],
      for (final ip in v4) ('your-domain.com', 'A', ip),
      for (final ip in v6) ('your-domain.com', 'AAAA', ip),
      if (o.baseHost.isNotEmpty) ('www.your-domain.com', 'CNAME', o.baseHost),
    ];
    return NCard(
      title: 'DNS setup',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (ips.isEmpty)
            Text('The server\'s public IP is unknown. Set it in the setup wizard (Settings → Domain, HTTPS email and sample app) so the records can be shown and checked.', style: TextStyle(color: c.warning))
          else
            Text('Point domains at this server with these records at your DNS provider:', style: TextStyle(color: c.muted, fontSize: 13)),
          const SizedBox(height: 8),
          for (final (name, type, value) in records)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 2),
              child: Row(
                children: [
                  Expanded(flex: 4, child: Text(name, style: monoStyle(context, size: 12.5))),
                  SizedBox(width: 64, child: Text(type, style: monoStyle(context, size: 12.5, color: c.muted))),
                  Expanded(flex: 4, child: SelectableText(value, style: monoStyle(context, size: 12.5))),
                  IconButton(
                    tooltip: 'Copy',
                    visualDensity: VisualDensity.compact,
                    icon: const Icon(Icons.copy_rounded, size: 15),
                    onPressed: () => Clipboard.setData(ClipboardData(text: value)),
                  ),
                ],
              ),
            ),
          if (o.baseDomain.isNotEmpty) ...[
            const SizedBox(height: 4),
            Text('The *.${o.baseDomain} record serves every app\'s default domain. A CNAME to ${o.baseHost} works for subdomains of your own domains once it exists.',
                style: TextStyle(color: c.muted, fontSize: 12.5)),
          ],
          const Divider(height: 24),
          SwitchListTile(
            contentPadding: EdgeInsets.zero,
            value: o.dnsAuto,
            onChanged: canManage && !_busy && (o.dnsAvailable || o.dnsAuto) ? _toggleAuto : null,
            title: const Text('Create DNS records automatically'),
            subtitle: Text(
              o.dnsAvailable
                  ? 'Through the gateway\'s DNS provider: one *.${o.baseDomain.isEmpty ? '<base domain>' : o.baseDomain} record, plus a record for each custom domain. Records someone else created are never changed.'
                  : 'Needs a DNS provider in the gateway (Gateway → Certificates → DNS provider) and nectar-gateway 1.3 or newer.',
              style: TextStyle(fontSize: 12.5, color: c.muted),
            ),
          ),
        ],
      ),
    );
  }
}

class _DomainTile extends ConsumerStatefulWidget {
  const _DomainTile({required this.row, required this.overview});

  final DomainRow row;
  final DomainsOverview overview;

  @override
  ConsumerState<_DomainTile> createState() => _DomainTileState();
}

class _DomainTileState extends ConsumerState<_DomainTile> {
  bool _busy = false;

  Future<void> _check() async {
    final d = widget.row.domain;
    setState(() => _busy = true);
    try {
      final res = await ref.read(apiProvider).post('/apps/${widget.row.appId}/domains/${d.id}/check');
      if (mounted && res is Map) showSnack(context, '${res['message']}', error: res['ok'] != true);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _createRecord() async {
    final d = widget.row.domain;
    setState(() => _busy = true);
    try {
      final res = await ref.read(apiProvider).post('/apps/${widget.row.appId}/domains/${d.id}/dns');
      ref.invalidate(domainsOverviewProvider);
      if (mounted && res is Map) showSnack(context, '${res['message']}', error: res['state'] == 'conflict');
    } on Object catch (e) {
      ref.invalidate(domainsOverviewProvider);
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final r = widget.row;
    final d = r.domain;
    final dnsText = switch (d.dnsState) {
      'managed' => 'DNS by Nectar',
      'covered' => 'DNS: *.${widget.overview.baseDomain}',
      'conflict' => 'DNS: existing records',
      'error' => 'DNS failed',
      _ => '',
    };
    final details = [
      '${r.projectName} / ${r.appName}',
      if (d.kind == 'redirect') 'redirect → ${d.redirectTo}',
      if (d.tls && d.certNotAfter != null) 'HTTPS until ${d.certNotAfter!.toLocal().toString().substring(0, 10)}' else if (d.tls) 'HTTPS pending' else 'HTTP only',
      if (d.web != null && !d.web!.isEmpty) d.web!.summary,
      if (dnsText.isNotEmpty) dnsText,
    ].join(' · ');
    final problem = [if (d.status == 'error' || d.statusMessage.isNotEmpty) d.statusMessage, if (d.dnsState == 'conflict' || d.dnsState == 'error') d.dnsMessage]
        .where((m) => m.isNotEmpty)
        .join(' · ');
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.only(top: 4),
            child: StatusDot(status: d.status),
          ),
          const SizedBox(width: 10),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Flexible(child: Text('${d.host}${d.pathPrefix == '/' ? '' : d.pathPrefix}', style: monoStyle(context, size: 13.5), overflow: TextOverflow.ellipsis)),
                    if (d.isDefault) ...[
                      const SizedBox(width: 6),
                      Container(
                        padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 1),
                        decoration: BoxDecoration(border: Border.all(color: c.muted.withValues(alpha: 0.5)), borderRadius: BorderRadius.circular(6)),
                        child: Text('default', style: TextStyle(color: c.muted, fontSize: 11)),
                      ),
                    ],
                  ],
                ),
                Text(details, style: TextStyle(color: c.muted, fontSize: 12)),
                if (problem.isNotEmpty) Text(problem, style: TextStyle(color: d.status == 'error' || d.dnsState == 'error' ? c.failed : c.warning, fontSize: 12)),
              ],
            ),
          ),
          if (_busy)
            const Padding(padding: EdgeInsets.all(10), child: SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2)))
          else
            PopupMenuButton<String>(
              tooltip: 'Actions',
              onSelected: (v) => switch (v) {
                'open' => context.go('/apps/${r.appId}'),
                'check' => _check(),
                'dns' => _createRecord(),
                'edit' => _edit(context),
                _ => null,
              },
              itemBuilder: (_) => [
                const PopupMenuItem(value: 'open', child: Text('Open app')),
                const PopupMenuItem(value: 'check', child: Text('Check DNS')),
                if (r.canManage && widget.overview.dnsAvailable && !d.host.startsWith('*.') && d.dnsState != 'covered')
                  PopupMenuItem(value: 'dns', child: Text(d.dnsState == 'managed' ? 'Update DNS record' : 'Create DNS record')),
                if (r.canManage) const PopupMenuItem(value: 'edit', child: Text('Edit')),
              ],
            ),
        ],
      ),
    );
  }

  Future<void> _edit(BuildContext context) async {
    await showDomainDialog(context, appId: widget.row.appId, domain: widget.row.domain);
    ref.invalidate(domainsOverviewProvider);
  }
}

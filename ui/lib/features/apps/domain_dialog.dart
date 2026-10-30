import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import 'web_options_editor.dart';

/// Add a domain to an app, or change its gateway options (HTTPS, rate limit, body size, timeout).
Future<void> showDomainDialog(BuildContext context, {required int appId, Domain? domain}) =>
    showDialog<void>(context: context, builder: (_) => _DomainDialog(appId: appId, domain: domain));

class _DomainDialog extends ConsumerStatefulWidget {
  const _DomainDialog({required this.appId, this.domain});

  final int appId;
  final Domain? domain;

  @override
  ConsumerState<_DomainDialog> createState() => _DomainDialogState();
}

class _DomainDialogState extends ConsumerState<_DomainDialog> {
  late final _host = TextEditingController(text: widget.domain?.host ?? '');
  late final _rate = TextEditingController(text: '${widget.domain?.rateLimit ?? 0}');
  late final _body = TextEditingController(text: '${widget.domain?.maxBodyMb ?? 0}');
  late final _timeout = TextEditingController(text: '${widget.domain?.timeoutSeconds ?? 0}');
  late bool _tls = widget.domain?.tls ?? true;
  late bool _force = widget.domain?.forceHttps ?? true;
  late final _web = WebOptionsForm(widget.domain?.web);
  bool _www = false;
  bool _busy = false;

  @override
  void dispose() {
    for (final c in [_host, _rate, _body, _timeout]) {
      c.dispose();
    }
    _web.dispose();
    super.dispose();
  }

  int _num(TextEditingController c) => int.tryParse(c.text.trim()) ?? -1;

  Future<void> _save() async {
    setState(() => _busy = true);
    final d = widget.domain;
    final body = {
      if (d == null) 'host': _host.text.trim(),
      if (d == null && _www) 'with_www': true,
      'tls': _tls,
      'force_https': _tls && _force,
      if (d == null || d.kind == 'app') ...{'rate_limit': _num(_rate), 'max_body_mb': _num(_body), 'timeout_seconds': _num(_timeout)},
      if ((d == null && !_web.value.isEmpty) || (d != null && _web.changed)) 'web': _web.value.toJson(),
    };
    try {
      final api = ref.read(apiProvider);
      if (d == null) {
        await api.post('/apps/${widget.appId}/domains', data: body);
      } else {
        await api.patch('/apps/${widget.appId}/domains/${d.id}', data: body);
      }
      ref.invalidate(domainsProvider(widget.appId));
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _delete() async {
    final d = widget.domain!;
    if (!await confirmAction(context, 'Remove ${d.host}?', 'The gateway stops serving it.', 'Remove', danger: true)) return;
    try {
      await ref.read(apiProvider).delete('/apps/${widget.appId}/domains/${d.id}');
      ref.invalidate(domainsProvider(widget.appId));
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final d = widget.domain;
    final appDomain = d == null || d.kind == 'app';
    return AlertDialog(
      title: Text(d == null ? 'Add domain' : d.host),
      content: SizedBox(
        width: 480,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (d == null) ...[
                TextField(controller: _host, decoration: const InputDecoration(labelText: 'Domain', hintText: 'shop.example.com')),
                const SizedBox(height: 4),
                Text('Point its DNS (A record) to this server first.', style: TextStyle(color: c.muted, fontSize: 12.5)),
                CheckboxListTile(
                  contentPadding: EdgeInsets.zero,
                  controlAffinity: ListTileControlAffinity.leading,
                  value: _www,
                  onChanged: (v) => setState(() => _www = v ?? false),
                  title: const Text('Also www. (redirects here)'),
                ),
              ],
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                value: _tls,
                onChanged: (v) => setState(() => _tls = v),
                title: const Text('HTTPS (Let\'s Encrypt)'),
              ),
              if (_tls)
                SwitchListTile(
                  contentPadding: EdgeInsets.zero,
                  value: _force,
                  onChanged: (v) => setState(() => _force = v),
                  title: const Text('Redirect HTTP to HTTPS'),
                ),
              if (appDomain) ...[
                const SizedBox(height: 6),
                const SectionLabel('Limits'),
                Wrap(
                  spacing: 12,
                  runSpacing: 10,
                  children: [
                    SizedBox(
                      width: 140,
                      child: TextField(controller: _rate, keyboardType: TextInputType.number, decoration: const InputDecoration(labelText: 'Requests/s per IP', isDense: true)),
                    ),
                    SizedBox(
                      width: 140,
                      child: TextField(controller: _body, keyboardType: TextInputType.number, decoration: const InputDecoration(labelText: 'Max upload MB', isDense: true)),
                    ),
                    SizedBox(
                      width: 140,
                      child: TextField(controller: _timeout, keyboardType: TextInputType.number, decoration: const InputDecoration(labelText: 'Timeout s', isDense: true)),
                    ),
                  ],
                ),
                const SizedBox(height: 6),
                Text('0 = no limit (timeout: 60 s). Above the rate limit a client gets 429 (Too Many Requests); one that keeps going is blocked for a while (Gateway → Blocked IPs). One page load can make dozens of requests. Older gateways block for 24 h at once and need at least 10.', style: TextStyle(color: c.muted, fontSize: 12.5)),
                const SizedBox(height: 6),
                Theme(
                  data: Theme.of(context).copyWith(dividerColor: Colors.transparent),
                  child: ExpansionTile(
                    tilePadding: EdgeInsets.zero,
                    childrenPadding: EdgeInsets.zero,
                    initiallyExpanded: !(d?.web?.isEmpty ?? true),
                    title: const Text('Redirects, access, headers'),
                    subtitle: Text(
                      _web.value.isEmpty ? 'Redirect rules, IP allowlist, headers, CORS, gzip, HSTS' : _web.value.summary,
                      style: TextStyle(color: c.muted, fontSize: 12.5),
                    ),
                    children: [WebOptionsFields(form: _web, onChanged: () => setState(() {}))],
                  ),
                ),
              ],
            ],
          ),
        ),
      ),
      actions: [
        if (d != null && !d.isDefault) TextButton(onPressed: _delete, style: TextButton.styleFrom(foregroundColor: c.failed), child: const Text('Remove')),
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
        FilledButton(onPressed: _busy ? null : _save, child: Text(d == null ? 'Add' : 'Save')),
      ],
    );
  }
}

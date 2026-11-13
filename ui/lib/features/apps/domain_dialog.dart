import 'package:material_ui/material_ui.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import 'web_options_editor.dart';

/// Add a domain to an app, or change its gateway options (HTTPS, rate limit, body size, timeout).
/// Without [appId] it is a proxy: a domain that goes straight to an address and port.
Future<void> showDomainDialog(
  BuildContext context, {
  int? appId,
  Domain? domain,
}) => showDialog<void>(
  context: context,
  builder: (_) => _DomainDialog(appId: appId, domain: domain),
);

class _DomainDialog extends ConsumerStatefulWidget {
  const _DomainDialog({required this.appId, this.domain});

  final int? appId; // null = proxy
  final Domain? domain;

  @override
  ConsumerState<_DomainDialog> createState() => _DomainDialogState();
}

class _DomainDialogState extends ConsumerState<_DomainDialog> {
  late final _host = TextEditingController(text: widget.domain?.host ?? '');
  late final _path = TextEditingController(
    text: widget.domain == null || widget.domain!.pathPrefix == '/'
        ? ''
        : widget.domain!.pathPrefix,
  );
  late bool _strip = widget.domain?.stripPrefix ?? false;
  late final _rate = TextEditingController(
    text: '${widget.domain?.rateLimit ?? 0}',
  );
  late final _body = TextEditingController(
    text: '${widget.domain?.maxBodyMb ?? 0}',
  );
  late final _timeout = TextEditingController(
    text: '${widget.domain?.timeoutSeconds ?? 0}',
  );
  late bool _tls = widget.domain?.tls ?? true;
  late bool _force = widget.domain?.forceHttps ?? true;
  late final _web = WebOptionsForm(widget.domain?.web);
  late final _target = TextEditingController(
    text: widget.domain?.targetHost ?? '127.0.0.1',
  );
  late final _targetPort = TextEditingController(
    text: (widget.domain?.targetPort ?? 0) > 0
        ? '${widget.domain!.targetPort}'
        : '',
  );
  late bool _targetHttps = widget.domain?.targetHttps ?? false;

  /// Proxies only: proxy | redirect (chosen when adding, fixed afterwards).
  late String _kind = widget.domain?.kind == 'redirect' ? 'redirect' : 'proxy';
  late final _redirectTo = TextEditingController(
    text: widget.domain?.redirectTo ?? '',
  );
  late int _redirectCode = widget.domain?.redirectCode ?? 301;
  bool _www = false;
  bool _busy = false;

  @override
  void dispose() {
    for (final c in [
      _host,
      _path,
      _rate,
      _body,
      _timeout,
      _target,
      _targetPort,
      _redirectTo,
    ]) {
      c.dispose();
    }
    _web.dispose();
    super.dispose();
  }

  int _num(TextEditingController c) => int.tryParse(c.text.trim()) ?? -1;

  bool get _proxy => widget.appId == null;

  /// API path of the domains: an app's, or the proxies.
  String get _base => _proxy ? '/proxies' : '/apps/${widget.appId}/domains';

  void _invalidate() {
    if (_proxy) {
      ref.invalidate(proxiesProvider);
    } else {
      ref.invalidate(domainsProvider(widget.appId!));
    }
    ref.invalidate(domainsOverviewProvider);
  }

  Future<void> _save() async {
    setState(() => _busy = true);
    final d = widget.domain;
    final host = _host.text.trim().toLowerCase();
    final path = _path.text.trim().isEmpty ? '/' : _path.text.trim();
    final body = {
      if (d == null || host != d.host) 'host': host,
      if (d == null || path != (d.pathPrefix.isEmpty ? '/' : d.pathPrefix))
        'path_prefix': path,
      if (path != '/' || (d?.stripPrefix ?? false))
        'strip_prefix': path != '/' && _strip,
      if (d == null && _www && !_proxy) 'with_www': true,
      'tls': _tls,
      'force_https': _tls && _force,
      if (_proxy && d == null) 'kind': _kind,
      if (_proxy && _kind == 'proxy') ...{
        'target_host': _target.text.trim().toLowerCase(),
        'target_port': _num(_targetPort),
        'target_https': _targetHttps,
      },
      if (_proxy && _kind == 'redirect') ...{
        'redirect_to': _redirectTo.text.trim(),
        'redirect_code': _redirectCode,
      },
      if ((d == null && !(_proxy && _kind == 'redirect')) ||
          d?.kind == 'app' ||
          d?.kind == 'proxy') ...{
        'rate_limit': _num(_rate),
        'max_body_mb': _num(_body),
        'timeout_seconds': _num(_timeout),
      },
      if ((d == null && !_web.value.isEmpty) || (d != null && _web.changed))
        'web': _web.value.toJson(),
    };
    try {
      final api = ref.read(apiProvider);
      if (d == null) {
        await api.post(_base, data: body);
      } else {
        await api.patch('$_base/${d.id}', data: body);
      }
      _invalidate();
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _delete() async {
    final s = S.of(context);
    final d = widget.domain!;
    if (!await confirmAction(
      context,
      s.appRemove(d.host),
      s.appTheGatewayStopsServingIt,
      s.backupRemove2,
      danger: true,
    )) {
      return;
    }
    try {
      await ref.read(apiProvider).delete('$_base/${d.id}');
      _invalidate();
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final d = widget.domain;
    final redirect = _proxy && _kind == 'redirect';
    final appDomain =
        (d == null && !redirect) || d?.kind == 'app' || d?.kind == 'proxy';
    final proxyTarget = _proxy && _kind == 'proxy';
    return AlertDialog(
      title: Text(d == null ? (_proxy ? s.proxiesAdd : 'Add domain') : d.host),
      content: SizedBox(
        width: 480,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              TextField(
                controller: _host,
                enabled: d == null || !d.isDefault,
                decoration: InputDecoration(
                  labelText: s.gatewayDomain,
                  hintText: 'shop.example.com',
                  helperText: d != null && d.isDefault
                      ? s.domainDefaultKeepsName
                      : null,
                ),
              ),
              if (_proxy && d == null) ...[
                const SizedBox(height: 12),
                SegmentedButton<String>(
                  segments: [
                    ButtonSegment(
                      value: 'proxy',
                      icon: const Icon(Icons.swap_horiz_rounded, size: 18),
                      label: Text(s.proxyKindProxy),
                    ),
                    ButtonSegment(
                      value: 'redirect',
                      icon: const Icon(Icons.turn_right_rounded, size: 18),
                      label: Text(s.proxyKindRedirect),
                    ),
                  ],
                  selected: {_kind},
                  onSelectionChanged: (v) => setState(() => _kind = v.first),
                ),
              ],
              if (redirect) ...[
                const SizedBox(height: 10),
                TextField(
                  controller: _redirectTo,
                  decoration: InputDecoration(
                    labelText: s.proxyRedirectTo,
                    hintText: 'https://example.com',
                    helperText: s.proxyRedirectToHelp,
                  ),
                ),
                const SizedBox(height: 10),
                DropdownButtonFormField<int>(
                  initialValue: _redirectCode,
                  decoration: InputDecoration(labelText: s.proxyRedirectCode),
                  items: [
                    DropdownMenuItem(
                      value: 301,
                      child: Text(s.proxyRedirect301),
                    ),
                    DropdownMenuItem(
                      value: 302,
                      child: Text(s.proxyRedirect302),
                    ),
                    DropdownMenuItem(
                      value: 307,
                      child: Text(s.proxyRedirect307),
                    ),
                    DropdownMenuItem(
                      value: 308,
                      child: Text(s.proxyRedirect308),
                    ),
                  ],
                  onChanged: (v) => setState(() => _redirectCode = v ?? 301),
                ),
              ],
              if (proxyTarget) ...[
                const SizedBox(height: 10),
                Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Expanded(
                      child: TextField(
                        controller: _target,
                        decoration: InputDecoration(
                          labelText: s.proxyTargetHost,
                          hintText: '127.0.0.1',
                        ),
                      ),
                    ),
                    const SizedBox(width: 12),
                    SizedBox(
                      width: 110,
                      child: TextField(
                        controller: _targetPort,
                        keyboardType: TextInputType.number,
                        decoration: InputDecoration(
                          labelText: s.proxyTargetPort,
                          hintText: '8080',
                        ),
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 4),
                Text(
                  s.proxyTargetHostHelp,
                  style: TextStyle(color: c.muted, fontSize: 12.5),
                ),
                SwitchListTile(
                  contentPadding: EdgeInsets.zero,
                  value: _targetHttps,
                  onChanged: (v) => setState(() => _targetHttps = v),
                  title: Text(s.proxyTargetHttps),
                  subtitle: Text(
                    s.proxyTargetHttpsHelp,
                    style: TextStyle(color: c.muted, fontSize: 12.5),
                  ),
                ),
              ],
              if (appDomain) ...[
                const SizedBox(height: 10),
                TextField(
                  controller: _path,
                  enabled: d == null || !d.isDefault,
                  decoration: InputDecoration(
                    labelText: s.domainPathPrefix,
                    hintText: '/api',
                    helperText: s.domainPathPrefixHelp,
                  ),
                  onChanged: (_) => setState(() {}),
                ),
                if (_path.text.trim().isNotEmpty && _path.text.trim() != '/')
                  SwitchListTile(
                    contentPadding: EdgeInsets.zero,
                    value: _strip,
                    onChanged: (v) => setState(() => _strip = v),
                    title: Text(s.domainStripPrefix),
                    subtitle: Text(
                      _strip
                          ? s.domainStripPrefixOn(_path.text.trim())
                          : s.domainStripPrefixOff(_path.text.trim()),
                      style: TextStyle(color: c.muted, fontSize: 12.5),
                    ),
                  ),
              ],
              if (d == null) ...[
                const SizedBox(height: 4),
                Text(
                  s.appPointItsDnsARecord,
                  style: TextStyle(color: c.muted, fontSize: 12.5),
                ),
                if (!_proxy)
                  CheckboxListTile(
                    contentPadding: EdgeInsets.zero,
                    controlAffinity: ListTileControlAffinity.leading,
                    value: _www,
                    onChanged: (v) => setState(() => _www = v ?? false),
                    title: Text(s.appAlsoWwwRedirectsHere),
                  ),
              ],
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                value: _tls,
                onChanged: (v) => setState(() => _tls = v),
                title: Text(s.appHttpsLetSEncrypt),
              ),
              if (_tls)
                SwitchListTile(
                  contentPadding: EdgeInsets.zero,
                  value: _force,
                  onChanged: (v) => setState(() => _force = v),
                  title: Text(s.appRedirectHttpToHttps),
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
                      child: TextField(
                        controller: _rate,
                        keyboardType: TextInputType.number,
                        decoration: InputDecoration(
                          labelText: s.appRequestsSPerIp,
                          isDense: true,
                        ),
                      ),
                    ),
                    SizedBox(
                      width: 140,
                      child: TextField(
                        controller: _body,
                        keyboardType: TextInputType.number,
                        decoration: InputDecoration(
                          labelText: s.appMaxUploadMb,
                          isDense: true,
                        ),
                      ),
                    ),
                    SizedBox(
                      width: 140,
                      child: TextField(
                        controller: _timeout,
                        keyboardType: TextInputType.number,
                        decoration: InputDecoration(
                          labelText: s.appTimeoutS,
                          isDense: true,
                        ),
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 6),
                Text(
                  s.app0NoLimitTimeout60,
                  style: TextStyle(color: c.muted, fontSize: 12.5),
                ),
                const SizedBox(height: 6),
                Theme(
                  data: Theme.of(
                    context,
                  ).copyWith(dividerColor: Colors.transparent),
                  child: ExpansionTile(
                    tilePadding: EdgeInsets.zero,
                    childrenPadding: EdgeInsets.zero,
                    initiallyExpanded: !(d?.web?.isEmpty ?? true),
                    title: Text(s.appRedirectsAccessHeaders),
                    subtitle: Text(
                      _web.value.isEmpty
                          ? 'Redirect rules, IP allowlist, headers, CORS, gzip, HSTS'
                          : _web.value.summary,
                      style: TextStyle(color: c.muted, fontSize: 12.5),
                    ),
                    children: [
                      WebOptionsFields(
                        form: _web,
                        onChanged: () => setState(() {}),
                      ),
                    ],
                  ),
                ),
              ],
            ],
          ),
        ),
      ),
      actions: [
        if (d != null && !d.isDefault)
          TextButton(
            onPressed: _delete,
            style: TextButton.styleFrom(foregroundColor: c.failed),
            child: Text(s.backupRemove2),
          ),
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(s.actionCancel),
        ),
        FilledButton(
          onPressed: _busy ? null : _save,
          child: Text(d == null ? 'Add' : 'Save'),
        ),
      ],
    );
  }
}

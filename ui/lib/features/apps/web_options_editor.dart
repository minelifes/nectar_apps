import 'package:flutter/material.dart';

import '../../api/models.dart';
import '../../generated/l10n.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';

/// Editable state of a domain's web options (redirect rules, allowlist, headers, CORS, gzip, HSTS).
class WebOptionsForm {
  WebOptionsForm(WebOptions? w) {
    final o = w ?? const WebOptions();
    for (final r in o.redirects) {
      rules.add(_RuleRow(r));
    }
    allowIps.text = o.allowIps.join('\n');
    responseHeaders.text = _headersText(o.responseHeaders);
    requestHeaders.text = _headersText(o.requestHeaders);
    cors = o.corsOrigins.isNotEmpty;
    corsOrigins.text = o.corsOrigins.join('\n');
    corsCredentials = o.corsCredentials;
    compress = o.compress;
    hsts = o.hsts;
    hstsSubdomains = o.hstsSubdomains;
    _initial = jsonKey;
  }

  final rules = <_RuleRow>[];
  final allowIps = TextEditingController();
  final responseHeaders = TextEditingController();
  final requestHeaders = TextEditingController();
  final corsOrigins = TextEditingController();
  bool cors = false;
  bool corsCredentials = false;
  bool compress = false;
  int hsts = 0;
  bool hstsSubdomains = false;
  late final String _initial;

  /// Whether anything changed since the form was opened.
  bool get changed => jsonKey != _initial;

  String get jsonKey => value.toJson().toString();

  WebOptions get value => WebOptions(
        redirects: [
          for (final r in rules)
            if (r.from.text.trim().isNotEmpty && r.to.text.trim().isNotEmpty)
              RedirectRule(from: r.from.text.trim(), to: r.to.text.trim(), code: r.code, prefix: r.prefix),
        ],
        allowIps: _lines(allowIps.text),
        responseHeaders: _parseHeaders(responseHeaders.text),
        requestHeaders: _parseHeaders(requestHeaders.text),
        corsOrigins: cors ? _lines(corsOrigins.text) : const [],
        corsCredentials: cors && corsCredentials,
        compress: compress,
        hsts: hsts,
        hstsSubdomains: hsts > 0 && hstsSubdomains,
      );

  void dispose() {
    for (final c in [allowIps, responseHeaders, requestHeaders, corsOrigins]) {
      c.dispose();
    }
    for (final r in rules) {
      r.dispose();
    }
  }

  static List<String> _lines(String t) => t.split(RegExp(r'[\n,]')).map((s) => s.trim()).where((s) => s.isNotEmpty).toList();

  static String _headersText(Map<String, String> h) => h.entries.map((e) => '${e.key}: ${e.value}').join('\n');

  static Map<String, String> _parseHeaders(String t) {
    final out = <String, String>{};
    for (final line in t.split('\n')) {
      final i = line.indexOf(':');
      if (i > 0) out[line.substring(0, i).trim()] = line.substring(i + 1).trim();
    }
    return out;
  }
}

class _RuleRow {
  _RuleRow([RedirectRule? r])
      : from = TextEditingController(text: r?.from ?? ''),
        to = TextEditingController(text: r?.to ?? ''),
        code = r?.code ?? 301,
        prefix = r?.prefix ?? false;

  final TextEditingController from;
  final TextEditingController to;
  int code;
  bool prefix;

  void dispose() {
    from.dispose();
    to.dispose();
  }
}

/// The fields of a [WebOptionsForm]; [onChanged] rebuilds the parent.
class WebOptionsFields extends StatelessWidget {
  const WebOptionsFields({super.key, required this.form, required this.onChanged});

  final WebOptionsForm form;
  final VoidCallback onChanged;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final hint = TextStyle(color: c.muted, fontSize: 12.5);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SectionLabel('Redirect rules'),
        for (final r in form.rules)
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: Wrap(
              spacing: 8,
              runSpacing: 6,
              crossAxisAlignment: WrapCrossAlignment.center,
              children: [
                SizedBox(width: 120, child: TextField(controller: r.from, decoration: InputDecoration(labelText: s.updateFrom, hintText: '/old', isDense: true))),
                SizedBox(
                  width: 190,
                  child: TextField(controller: r.to, decoration: InputDecoration(labelText: s.backupTo, hintText: s.appNewOrHttps, isDense: true)),
                ),
                DropdownButton<int>(
                  value: r.code,
                  isDense: true,
                  items: const [
                    DropdownMenuItem(value: 301, child: Text('301')),
                    DropdownMenuItem(value: 302, child: Text('302')),
                    DropdownMenuItem(value: 307, child: Text('307')),
                    DropdownMenuItem(value: 308, child: Text('308')),
                  ],
                  onChanged: (v) {
                    r.code = v ?? 301;
                    onChanged();
                  },
                ),
                Tooltip(
                  message: s.appAlsoRedirectFromAndKeep,
                  child: Row(mainAxisSize: MainAxisSize.min, children: [
                    Checkbox(
                      value: r.prefix,
                      visualDensity: VisualDensity.compact,
                      onChanged: (v) {
                        r.prefix = v ?? false;
                        onChanged();
                      },
                    ),
                    const Text('sub-paths', style: TextStyle(fontSize: 12.5)),
                  ]),
                ),
                IconButton(
                  tooltip: s.appRemoveRule,
                  visualDensity: VisualDensity.compact,
                  icon: const Icon(Icons.close_rounded, size: 18),
                  onPressed: () {
                    form.rules.remove(r);
                    r.dispose();
                    onChanged();
                  },
                ),
              ],
            ),
          ),
        Align(
          alignment: Alignment.centerLeft,
          child: TextButton.icon(
            onPressed: () {
              form.rules.add(_RuleRow());
              onChanged();
            },
            icon: const Icon(Icons.add_rounded, size: 18),
            label: Text(s.appAddRedirectRule),
          ),
        ),
        const SizedBox(height: 8),
        const SectionLabel('Access'),
        TextField(
          controller: form.allowIps,
          minLines: 1,
          maxLines: 4,
          decoration: InputDecoration(labelText: s.appAllowedIps, hintText: '203.0.113.0/24', isDense: true),
        ),
        const SizedBox(height: 4),
        Text(s.appOneIpOrCidrPer, style: hint),
        const SizedBox(height: 12),
        const SectionLabel('Headers'),
        TextField(
          controller: form.responseHeaders,
          minLines: 1,
          maxLines: 5,
          decoration: InputDecoration(labelText: s.appResponseHeaders, hintText: s.appXFrameOptionsDeny, isDense: true),
        ),
        const SizedBox(height: 8),
        TextField(
          controller: form.requestHeaders,
          minLines: 1,
          maxLines: 4,
          decoration: InputDecoration(labelText: s.appRequestHeadersToTheApp, hintText: s.appXEnvStaging, isDense: true),
        ),
        const SizedBox(height: 4),
        Text(s.appNameValuePerLineAn, style: hint),
        SwitchListTile(
          contentPadding: EdgeInsets.zero,
          value: form.cors,
          onChanged: (v) {
            form.cors = v;
            onChanged();
          },
          title: const Text('CORS'),
          subtitle: Text(s.appLetOtherSitesCallThis),
        ),
        if (form.cors) ...[
          TextField(
            controller: form.corsOrigins,
            minLines: 1,
            maxLines: 4,
            decoration: InputDecoration(labelText: s.appAllowedOrigins, hintText: 'https://app.example.com', isDense: true),
          ),
          const SizedBox(height: 4),
          Text(s.appOnePerLineOrFor, style: hint),
          CheckboxListTile(
            contentPadding: EdgeInsets.zero,
            controlAffinity: ListTileControlAffinity.leading,
            value: form.corsCredentials,
            onChanged: (v) {
              form.corsCredentials = v ?? false;
              onChanged();
            },
            title: Text(s.appAllowCookiesCredentials),
          ),
        ],
        SwitchListTile(
          contentPadding: EdgeInsets.zero,
          value: form.compress,
          onChanged: (v) {
            form.compress = v;
            onChanged();
          },
          title: Text(s.appCompressTextResponsesGzip),
          subtitle: Text(s.appHtmlCssJsJsonThat),
        ),
        DropdownButtonFormField<int>(
          initialValue: const [0, 300, 86400, 15768000, 31536000, 63072000].contains(form.hsts) ? form.hsts : 31536000,
          decoration: InputDecoration(labelText: s.appHstsBrowsersOnlyUseHttps, isDense: true),
          items: [
            DropdownMenuItem(value: 0, child: Text(s.appOff)),
            DropdownMenuItem(value: 300, child: Text(s.app5MinutesTesting)),
            DropdownMenuItem(value: 86400, child: Text(s.gateway1Day)),
            DropdownMenuItem(value: 15768000, child: Text(s.app6Months)),
            DropdownMenuItem(value: 31536000, child: Text(s.app1Year)),
            DropdownMenuItem(value: 63072000, child: Text(s.app2Years)),
          ],
          onChanged: (v) {
            form.hsts = v ?? 0;
            onChanged();
          },
        ),
        if (form.hsts > 0)
          CheckboxListTile(
            contentPadding: EdgeInsets.zero,
            controlAffinity: ListTileControlAffinity.leading,
            value: form.hstsSubdomains,
            onChanged: (v) {
              form.hstsSubdomains = v ?? false;
              onChanged();
            },
            title: Text(s.appIncludeSubdomains),
          ),
        const SizedBox(height: NectarTokens.gap / 2),
      ],
    );
  }
}

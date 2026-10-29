import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../theme/tokens.dart';

IconData channelIcon(String kind) => switch (kind) {
      'ntfy' => Icons.notifications_active_outlined,
      'telegram' => Icons.send_outlined,
      'email' => Icons.mail_outline_rounded,
      _ => Icons.webhook_rounded,
    };

String channelLabel(String kind) => switch (kind) {
      'ntfy' => 'ntfy',
      'telegram' => 'Telegram',
      'email' => 'Email',
      'webhook' => 'Webhook',
      _ => kind,
    };

/// Adds a channel, or edits [channel].
Future<void> showChannelDialog(BuildContext context, WidgetRef ref, {AlertChannel? channel}) =>
    showDialog<void>(context: context, builder: (_) => _ChannelDialog(channel: channel));

class _Field {
  const _Field(this.key, this.label, {this.hint, this.help, this.secret = false, this.number = false});

  final String key;
  final String label;
  final String? hint;
  final String? help;
  final bool secret;
  final bool number;
}

const _fields = <String, List<_Field>>{
  'ntfy': [
    _Field('url', 'Server', hint: 'https://ntfy.sh', help: 'Your own ntfy server or ntfy.sh'),
    _Field('topic', 'Topic', hint: 'nectar-alerts-7f3k', help: 'On ntfy.sh anyone who knows the topic can read it: pick a hard-to-guess name'),
    _Field('token', 'Access token (optional)', secret: true),
  ],
  'telegram': [
    _Field('bot_token', 'Bot token', hint: '123456789:AA…', help: 'Create a bot with @BotFather and paste its token', secret: true),
    _Field('chat_id', 'Chat id', hint: '123456789 or @channel', help: 'Send the bot a message, then read your id from @userinfobot'),
  ],
  'email': [
    _Field('host', 'SMTP server', hint: 'smtp.example.com'),
    _Field('port', 'Port', hint: '587', number: true),
    _Field('username', 'User name (optional)'),
    _Field('password', 'Password', secret: true),
    _Field('from', 'From', hint: 'Nectar Apps <alerts@example.com>'),
    _Field('to', 'To', hint: 'ops@example.com, me@example.com', help: 'Comma-separated'),
  ],
  'webhook': [
    _Field('url', 'URL', hint: 'https://example.com/hooks/nectar', help: 'Receives a JSON POST for every alert'),
    _Field('secret', 'Signing secret (optional)', help: 'Signs the body: X-Nectar-Signature: sha256=<HMAC>', secret: true),
  ],
};

class _ChannelDialog extends ConsumerStatefulWidget {
  const _ChannelDialog({this.channel});

  final AlertChannel? channel;

  @override
  ConsumerState<_ChannelDialog> createState() => _ChannelDialogState();
}

class _ChannelDialogState extends ConsumerState<_ChannelDialog> {
  late String _kind = widget.channel?.kind ?? 'ntfy';
  late final _name = TextEditingController(text: widget.channel?.name ?? '');
  final Map<String, TextEditingController> _ctl = {};
  late String _security = '${widget.channel?.config['security'] ?? 'starttls'}';
  late bool _panelSmtp = widget.channel?.config['use_panel_smtp'] == true || widget.channel == null;

  /// Email channels can use the panel's SMTP server (Settings → Email): then only the recipients matter.
  bool _shown(_Field f) => !(_kind == 'email' && _panelSmtp && f.key != 'to');
  bool _saving = false;
  String? _error;

  TextEditingController _c(String key) => _ctl.putIfAbsent(key, () {
        final v = widget.channel?.config[key];
        return TextEditingController(text: v == null || v == 0 ? '' : '$v');
      });

  @override
  void dispose() {
    _name.dispose();
    for (final c in _ctl.values) {
      c.dispose();
    }
    super.dispose();
  }

  Future<void> _save() async {
    final config = <String, dynamic>{};
    for (final f in _fields[_kind]!.where(_shown)) {
      final v = _c(f.key).text.trim();
      if (f.number) {
        if (v.isNotEmpty) config[f.key] = int.tryParse(v) ?? -1;
      } else {
        config[f.key] = v;
      }
    }
    if (_kind == 'email' && _panelSmtp) config['use_panel_smtp'] = true;
    if (_kind == 'email' && !_panelSmtp) config['security'] = _security;
    final name = _name.text.trim().isEmpty ? channelLabel(_kind) : _name.text.trim();
    setState(() {
      _saving = true;
      _error = null;
    });
    final api = ref.read(apiProvider);
    try {
      final ch = widget.channel;
      if (ch == null) {
        await api.post('/alerts/channels', data: {'kind': _kind, 'name': name, 'config': config});
      } else {
        await api.put('/alerts/channels/${ch.id}', data: {'name': name, 'enabled': ch.enabled, 'config': config});
      }
      ref.invalidate(alertChannelsProvider);
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final editing = widget.channel != null;
    return AlertDialog(
      title: Text(editing ? 'Edit ${widget.channel!.name}' : 'Add a channel'),
      content: SizedBox(
        width: 480,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (!editing) ...[
                SegmentedButton<String>(
                  segments: [
                    for (final k in _fields.keys) ButtonSegment(value: k, icon: Icon(channelIcon(k), size: 16), label: Text(channelLabel(k))),
                  ],
                  selected: {_kind},
                  showSelectedIcon: false,
                  onSelectionChanged: (s) => setState(() => _kind = s.first),
                ),
                const SizedBox(height: 16),
              ],
              TextField(controller: _name, decoration: InputDecoration(labelText: 'Name', hintText: channelLabel(_kind))),
              if (_kind == 'email')
                SwitchListTile(
                  contentPadding: EdgeInsets.zero,
                  value: _panelSmtp,
                  onChanged: (v) => setState(() => _panelSmtp = v),
                  title: const Text('Send with the panel\'s email settings'),
                  subtitle: const Text('Settings → Email; turn off to use another SMTP server'),
                ),
              for (final f in _fields[_kind]!.where(_shown)) ...[
                const SizedBox(height: 12),
                TextField(
                  controller: _c(f.key),
                  obscureText: f.secret && _c(f.key).text != maskedSecret,
                  keyboardType: f.number ? TextInputType.number : null,
                  decoration: InputDecoration(
                    labelText: f.label,
                    hintText: f.hint,
                    helperText: f.secret && _c(f.key).text == maskedSecret ? 'Saved. Replace it to change it.' : f.help,
                    helperMaxLines: 2,
                  ),
                  onTap: f.secret && _c(f.key).text == maskedSecret ? () => setState(() => _c(f.key).clear()) : null,
                ),
              ],
              if (_kind == 'email' && !_panelSmtp) ...[
                const SizedBox(height: 12),
                Row(
                  children: [
                    const Text('Encryption'),
                    const SizedBox(width: 12),
                    DropdownButton<String>(
                      value: _security,
                      items: const [
                        DropdownMenuItem(value: 'starttls', child: Text('STARTTLS (port 587)')),
                        DropdownMenuItem(value: 'tls', child: Text('TLS (port 465)')),
                        DropdownMenuItem(value: 'none', child: Text('None (port 25, local relays only)')),
                      ],
                      onChanged: (v) => setState(() => _security = v ?? 'starttls'),
                    ),
                  ],
                ),
              ],
              if (_error != null)
                Padding(padding: const EdgeInsets.only(top: 12), child: Text(_error!, style: TextStyle(color: c.failed))),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
        FilledButton(onPressed: _saving ? null : _save, child: Text(_saving ? 'Saving…' : 'Save')),
      ],
    );
  }
}

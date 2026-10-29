import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';

const _masked = '••••••••';

/// Adds a backup target, or edits [target].
Future<void> showTargetDialog(BuildContext context, WidgetRef ref, {BackupTarget? target}) =>
    showDialog<void>(context: context, builder: (_) => _TargetDialog(target: target));

class _TargetDialog extends ConsumerStatefulWidget {
  const _TargetDialog({this.target});

  final BackupTarget? target;

  @override
  ConsumerState<_TargetDialog> createState() => _TargetDialogState();
}

class _TargetDialogState extends ConsumerState<_TargetDialog> {
  // b2 is s3 with a Backblaze endpoint
  late String _kind = widget.target?.kind ?? 's3';
  late bool _b2 = '${widget.target?.config['endpoint'] ?? ''}'.contains('backblazeb2.com');
  late final _name = TextEditingController(text: widget.target?.name ?? '');
  final Map<String, TextEditingController> _ctl = {};
  bool _saving = false;
  String? _error;

  TextEditingController _c(String key) => _ctl.putIfAbsent(key, () {
        final v = widget.target?.config[key];
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
    final keys = switch (_kind) {
      's3' => ['endpoint', 'bucket', 'prefix', 'region', 'access_key', 'secret_key', 'password'],
      'sftp' => ['host', 'port', 'user', 'path', 'private_key', 'password'],
      _ => ['path', 'password'],
    };
    final cfg = <String, dynamic>{};
    for (final k in keys) {
      final v = _c(k).text.trim();
      if (k == 'port') {
        if (v.isNotEmpty) cfg[k] = int.tryParse(v) ?? 0;
      } else if (v.isNotEmpty) {
        cfg[k] = v;
      }
    }
    final name = _name.text.trim().isEmpty ? (_b2 ? 'Backblaze B2' : _kind.toUpperCase()) : _name.text.trim();
    setState(() {
      _saving = true;
      _error = null;
    });
    final api = ref.read(apiProvider);
    try {
      final t = widget.target;
      if (t == null) {
        await api.post('/backup/targets', data: {'name': name, 'kind': _kind, 'config': cfg});
      } else {
        await api.put('/backup/targets/${t.id}', data: {'name': name, 'config': cfg});
      }
      ref.invalidate(backupTargetsProvider);
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  Widget _field(String key, String label, {String? hint, String? help, bool secret = false, bool multiline = false, bool number = false}) {
    final ctl = _c(key);
    final kept = ctl.text == _masked;
    return Padding(
      padding: const EdgeInsets.only(top: 12),
      child: TextField(
        controller: ctl,
        obscureText: secret && !multiline && !kept,
        minLines: multiline ? 4 : 1,
        maxLines: multiline ? 8 : 1,
        keyboardType: number ? TextInputType.number : null,
        inputFormatters: number ? [FilteringTextInputFormatter.digitsOnly] : null,
        style: multiline ? monoStyle(context, size: 12) : null,
        decoration: InputDecoration(labelText: label, hintText: hint, helperText: kept ? 'Saved. Replace it to change it.' : help, helperMaxLines: 3),
        onTap: kept ? () => setState(ctl.clear) : null,
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final editing = widget.target != null;
    final fields = <Widget>[
      if (_kind == 's3') ...[
        _field('endpoint', 'Endpoint',
            hint: _b2 ? 'https://s3.us-west-004.backblazeb2.com' : 'https://s3.eu-central-1.amazonaws.com',
            help: _b2 ? 'The S3 endpoint shown on your bucket page' : 'Empty = AWS; R2, MinIO, Wasabi: their S3 endpoint'),
        _field('bucket', 'Bucket'),
        _field('prefix', 'Folder in the bucket (optional)', hint: 'nectar'),
        if (!_b2) _field('region', 'Region (optional)', hint: 'eu-central-1'),
        _field('access_key', _b2 ? 'Key ID' : 'Access key'),
        _field('secret_key', _b2 ? 'Application key' : 'Secret key', secret: true),
      ],
      if (_kind == 'sftp') ...[
        _field('host', 'Host', hint: 'backup.example.com'),
        _field('port', 'Port', hint: '22', number: true),
        _field('user', 'User'),
        _field('path', 'Folder on the server', hint: '/srv/backups/nectar'),
        _field('private_key', 'Private key (OpenSSH, no passphrase)', hint: '-----BEGIN OPENSSH PRIVATE KEY-----', secret: true, multiline: true),
      ],
      if (_kind == 'local') ...[
        _field('path', 'Folder on this server', hint: '/var/backups/nectar',
            help: 'Survives redeploys but not a lost server: also back up to a remote target for disaster recovery'),
      ],
      if (!editing)
        _field('password', 'Repository password (optional)',
            secret: true, help: 'Empty = a strong random one. To use an existing restic repository, enter its password.'),
    ];
    return AlertDialog(
      title: Text(editing ? 'Edit ${widget.target!.name}' : 'Add a backup target'),
      content: SizedBox(
        width: 500,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (!editing)
                SegmentedButton<String>(
                  segments: const [
                    ButtonSegment(value: 's3', label: Text('S3')),
                    ButtonSegment(value: 'b2', label: Text('Backblaze B2')),
                    ButtonSegment(value: 'sftp', label: Text('SFTP')),
                    ButtonSegment(value: 'local', label: Text('Folder')),
                  ],
                  selected: {_b2 ? 'b2' : _kind},
                  showSelectedIcon: false,
                  onSelectionChanged: (s) => setState(() {
                    _b2 = s.first == 'b2';
                    _kind = _b2 ? 's3' : s.first;
                  }),
                ),
              const SizedBox(height: 4),
              TextField(controller: _name, decoration: InputDecoration(labelText: 'Name', hintText: _b2 ? 'Backblaze B2' : _kind.toUpperCase())),
              ...fields,
              const SizedBox(height: 12),
              Text('Backups are encrypted with the repository password before they leave the server.', style: TextStyle(color: c.muted, fontSize: 12)),
              if (_error != null) Padding(padding: const EdgeInsets.only(top: 12), child: Text(_error!, style: TextStyle(color: c.failed))),
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


import 'package:flutter/material.dart';

import '../../api/models.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import 'resources.dart';

/// Editable list of environment variables (mutates [vars] in place and calls [onChanged]).
/// Secret values are hidden; a secret loaded from the server without its value is kept unless replaced.
class EnvEditor extends StatelessWidget {
  const EnvEditor({super.key, required this.vars, required this.onChanged});

  final List<EnvVar> vars;
  final VoidCallback onChanged;

  void _add() {
    vars.add(EnvVar(key: '', value: ''));
    onChanged();
  }

  Future<void> _paste(BuildContext context) async {
    final s = S.of(context);
    final ctl = TextEditingController();
    final text = await showDialog<String>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(s.appPasteAEnvFile),
        content: SizedBox(
          width: 520,
          child: TextField(
            controller: ctl,
            autofocus: true,
            maxLines: 12,
            minLines: 8,
            style: monoStyle(ctx, size: 13),
            decoration: InputDecoration(
              hintText: s.appDatabaseUrlPostgresLogLevel,
            ),
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx),
            child: Text(s.actionCancel),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(ctx, ctl.text),
            child: Text(s.appAddVariables),
          ),
        ],
      ),
    );
    ctl.dispose();
    if (text == null) return;
    for (final (key, value) in parseDotEnv(text)) {
      final existing = vars.where((v) => v.key == key).firstOrNull;
      if (existing != null) {
        existing.value = value;
      } else {
        vars.add(EnvVar(key: key, value: value, secret: looksSecret(key)));
      }
    }
    vars.removeWhere((v) => v.key.isEmpty && (v.value ?? '').isEmpty);
    onChanged();
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (vars.isEmpty)
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: Text(
              s.appNoVariablesEveryAppAlso,
              style: TextStyle(color: c.muted, fontSize: 13),
            ),
          ),
        for (final v in vars)
          _EnvRow(
            key: ObjectKey(v),
            v: v,
            onChanged: onChanged,
            onRemove: () {
              vars.remove(v);
              onChanged();
            },
          ),
        const SizedBox(height: 6),
        Wrap(
          spacing: 8,
          children: [
            TextButton.icon(
              onPressed: _add,
              icon: const Icon(Icons.add_rounded, size: 18),
              label: Text(s.appAddVariable),
            ),
            TextButton.icon(
              onPressed: () => _paste(context),
              icon: const Icon(Icons.content_paste_rounded, size: 18),
              label: Text(s.appPasteEnv),
            ),
          ],
        ),
      ],
    );
  }
}

class _EnvRow extends StatefulWidget {
  const _EnvRow({
    super.key,
    required this.v,
    required this.onChanged,
    required this.onRemove,
  });

  final EnvVar v;
  final VoidCallback onChanged;
  final VoidCallback onRemove;

  @override
  State<_EnvRow> createState() => _EnvRowState();
}

class _EnvRowState extends State<_EnvRow> {
  late final _key = TextEditingController(text: widget.v.key);
  late final _value = TextEditingController(text: widget.v.value ?? '');
  bool _show = false;

  @override
  void didUpdateWidget(_EnvRow old) {
    super.didUpdateWidget(old);
    // a pasted .env may have replaced the value
    final v = widget.v.value ?? '';
    if (v != _value.text && widget.v.value != null) _value.text = v;
  }

  @override
  void dispose() {
    _key.dispose();
    _value.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final v = widget.v;
    final kept = v.secret && v.value == null;
    final c = NectarColors.of(context);
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Row(
        children: [
          Expanded(
            flex: 4,
            child: TextField(
              controller: _key,
              style: monoStyle(context, size: 13.5),
              decoration: InputDecoration(hintText: s.appKey, isDense: true),
              onChanged: (t) {
                v.key = t.trim();
                widget.onChanged();
              },
            ),
          ),
          const SizedBox(width: 8),
          Expanded(
            flex: 6,
            child: TextField(
              controller: _value,
              obscureText: v.secret && !_show,
              style: monoStyle(context, size: 13.5),
              decoration: InputDecoration(
                hintText: kept ? '•••••• (unchanged)' : 'value',
                isDense: true,
                suffixIcon: v.secret && !kept
                    ? IconButton(
                        tooltip: _show ? 'Hide' : 'Show',
                        icon: Icon(
                          _show
                              ? Icons.visibility_off_outlined
                              : Icons.visibility_outlined,
                          size: 18,
                        ),
                        onPressed: () => setState(() => _show = !_show),
                      )
                    : null,
              ),
              onChanged: (t) {
                v.value = (kept && t.isEmpty) ? null : t;
                widget.onChanged();
              },
            ),
          ),
          const SizedBox(width: 4),
          Tooltip(
            message: v.secret
                ? 'Secret: stored encrypted, hidden in the UI'
                : 'Mark as secret',
            child: IconButton(
              icon: Icon(
                v.secret ? Icons.lock_rounded : Icons.lock_open_rounded,
                size: 18,
                color: v.secret ? NectarTokens.amber : c.muted,
              ),
              onPressed: () {
                setState(() => v.secret = !v.secret);
                widget.onChanged();
              },
            ),
          ),
          IconButton(
            tooltip: s.backupRemove2,
            icon: const Icon(Icons.close_rounded, size: 18),
            onPressed: widget.onRemove,
          ),
        ],
      ),
    );
  }
}

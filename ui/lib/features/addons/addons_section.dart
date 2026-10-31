import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/status.dart';

IconData addonIcon(String kind) => kind == 'redis' ? Icons.bolt_rounded : Icons.storage_rounded;

/// The databases of a project (project page).
class AddonsSection extends ConsumerWidget {
  const AddonsSection({super.key, required this.projectId, required this.canCreate});

  final int projectId;
  final bool canCreate;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final addons = ref.watch(projectAddonsProvider(projectId));
    final c = NectarColors.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        SectionLabel(
          'Databases',
          trailing: canCreate
              ? TextButton.icon(
                  onPressed: () => showCreateAddon(context, ref, projectId),
                  icon: const Icon(Icons.add_rounded, size: 18),
                  label: Text(s.addonAddDatabase),
                )
              : null,
        ),
        AsyncBlock(
          value: addons,
          loadingHeight: 80,
          onRetry: () => ref.invalidate(projectAddonsProvider(projectId)),
          data: (list) {
            if (list.isEmpty) {
              return NCard(
                child: Text(
                  s.addonPostgresqlMysqlMariadbOrRedis,
                  style: TextStyle(color: c.muted, fontSize: 13),
                ),
              );
            }
            return NCard(
              padding: const EdgeInsets.symmetric(vertical: 6),
              child: Column(
                children: [
                  for (var i = 0; i < list.length; i++) ...[
                    if (i > 0) const Divider(height: 1),
                    ListTile(
                      leading: Icon(addonIcon(list[i].kind), color: NectarTokens.amber),
                      title: Text(list[i].name, style: const TextStyle(fontWeight: FontWeight.w600)),
                      subtitle: Text(
                        '${list[i].kindName} ${list[i].version} · ${list[i].host}:${list[i].port} · ${formatMb(list[i].sizeMb)}'
                        '${list[i].links.isEmpty ? '' : ' · ${list[i].links.length} linked'}',
                        style: TextStyle(color: c.muted, fontSize: 12.5),
                      ),
                      trailing: StatusPill(status: list[i].liveStatus),
                      onTap: () => context.go('/addons/${list[i].id}'),
                    ),
                  ],
                ],
              ),
            );
          },
        ),
      ],
    );
  }
}

/// Asks for kind, version and size, creates the add-on and opens it.
Future<void> showCreateAddon(BuildContext context, WidgetRef ref, int projectId) =>
    showDialog<void>(context: context, builder: (_) => _CreateAddonDialog(projectId: projectId));

class _CreateAddonDialog extends ConsumerStatefulWidget {
  const _CreateAddonDialog({required this.projectId});

  final int projectId;

  @override
  ConsumerState<_CreateAddonDialog> createState() => _CreateAddonDialogState();
}

class _CreateAddonDialogState extends ConsumerState<_CreateAddonDialog> {
  String _kind = 'postgres';
  String? _version;
  final _slug = TextEditingController(text: 'postgres');
  final _size = TextEditingController(text: '2048');
  final _mem = TextEditingController(text: '256');
  bool _slugTouched = false;
  bool _saving = false;
  String? _error;

  @override
  void dispose() {
    _slug.dispose();
    _size.dispose();
    _mem.dispose();
    super.dispose();
  }

  void _pick(AddonKindInfo k) {
    setState(() {
      _kind = k.id;
      _version = k.defaultVersion;
      if (!_slugTouched) _slug.text = k.id;
      final mem = int.tryParse(_mem.text) ?? 0;
      if (mem < k.minMemoryMb) _mem.text = '${k.minMemoryMb < 256 ? 256 : k.minMemoryMb}';
      if (k.id == 'redis' && _size.text == '2048') _size.text = '512';
    });
  }

  Future<void> _create() async {
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      final res = await ref.read(apiProvider).post('/projects/${widget.projectId}/addons', data: {
        'kind': _kind,
        'slug': _slug.text.trim(),
        if (_version != null) 'version': _version,
        'size_mb': int.tryParse(_size.text.trim()) ?? 0,
        'memory_mb': int.tryParse(_mem.text.trim()) ?? 0,
      });
      ref.invalidate(projectAddonsProvider(widget.projectId));
      if (!mounted) return;
      Navigator.pop(context);
      context.go('/addons/${Addon.fromJson(res).id}');
    } on Object catch (e) {
      if (mounted) setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final kinds = ref.watch(addonKindsProvider);
    final c = NectarColors.of(context);
    return AlertDialog(
      title: Text(s.addonAddADatabase),
      content: SizedBox(
        width: 480,
        child: AsyncBlock(
          value: kinds,
          loadingHeight: 120,
          data: (list) {
            final k = list.where((x) => x.id == _kind).firstOrNull ?? list.first;
            final version = _version ?? k.defaultVersion;
            return Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  children: [
                    for (final x in list)
                      ChoiceChip(
                        avatar: Icon(addonIcon(x.id), size: 16),
                        label: Text(x.name),
                        selected: x.id == _kind,
                        onSelected: (_) => _pick(x),
                      ),
                  ],
                ),
                const SizedBox(height: 16),
                Row(
                  children: [
                    Expanded(
                      child: TextField(
                        controller: _slug,
                        decoration: InputDecoration(labelText: s.addonNameAlsoTheHostName, helperText: s.addonLowercaseLettersDigitsDashes),
                        onChanged: (_) => _slugTouched = true,
                      ),
                    ),
                    const SizedBox(width: 12),
                    if (k.versions.length > 1)
                      DropdownButton<String>(
                        value: version,
                        items: [for (final v in k.versions) DropdownMenuItem(value: v.$1, child: Text('${k.name} ${v.$1}'))],
                        onChanged: (v) => setState(() => _version = v),
                      )
                    else
                      Text('${k.name} $version'),
                  ],
                ),
                const SizedBox(height: 12),
                Row(
                  children: [
                    Expanded(
                      child: TextField(
                        controller: _size,
                        keyboardType: TextInputType.number,
                        inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                        decoration: InputDecoration(labelText: s.addonStorageMib),
                      ),
                    ),
                    const SizedBox(width: 12),
                    Expanded(
                      child: TextField(
                        controller: _mem,
                        keyboardType: TextInputType.number,
                        inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                        decoration: InputDecoration(labelText: s.addonMemoryMib, helperText: s.addonAtLeast(k.minMemoryMb)),
                      ),
                    ),
                  ],
                ),
                if (_error != null) Padding(padding: const EdgeInsets.only(top: 12), child: Text(_error!, style: TextStyle(color: c.failed))),
              ],
            );
          },
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: Text(s.actionCancel)),
        FilledButton(onPressed: _saving ? null : _create, child: Text(_saving ? 'Creating…' : 'Create')),
      ],
    );
  }
}

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';

/// Settings → Roles: the built-in roles (read-only, can be cloned) and custom roles with a permission matrix.
class RolesPage extends ConsumerWidget {
  const RolesPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final session = ref.watch(sessionProvider);
    final canEdit = session.can('role.manage');
    final roles = ref.watch(rolesProvider);
    final catalog = ref.watch(permissionCatalogProvider);
    final c = NectarColors.of(context);
    return PageScaffold(
      title: 'Roles',
      subtitle: 'A role is a set of permissions. Users get roles everywhere or for one project; API tokens can be limited to a role.',
      maxWidth: 980,
      onRefresh: () async => ref.invalidate(rolesProvider),
      actions: [
        if (canEdit && catalog.hasValue)
          FilledButton.icon(
            onPressed: () => showRoleEditor(context, ref, catalog: catalog.value!),
            icon: const Icon(Icons.add_rounded, size: 18),
            label: const Text('New role'),
          ),
      ],
      children: [
        NCard(
          padding: const EdgeInsets.symmetric(vertical: 6),
          child: AsyncBlock(
            value: roles,
            onRetry: () => ref.invalidate(rolesProvider),
            data: (list) => Column(
              children: [
                for (var i = 0; i < list.length; i++) ...[
                  if (i > 0) const Divider(height: 1),
                  ListTile(
                    leading: Icon(list[i].builtin ? Icons.shield_outlined : Icons.tune_rounded, color: list[i].builtin ? c.muted : NectarTokens.amber),
                    title: Text(list[i].name, style: const TextStyle(fontWeight: FontWeight.w600)),
                    subtitle: Text(
                      [
                        if (list[i].description.isNotEmpty) list[i].description,
                        '${list[i].permissions.length} permissions',
                        '${list[i].bindings} ${list[i].bindings == 1 ? 'assignment' : 'assignments'}',
                        if (list[i].builtin) 'built-in',
                      ].join(' · '),
                      style: TextStyle(fontSize: 12.5, color: c.muted),
                    ),
                    trailing: const Icon(Icons.chevron_right_rounded),
                    onTap: catalog.hasValue ? () => showRoleEditor(context, ref, catalog: catalog.value!, role: list[i], readOnly: !canEdit) : null,
                  ),
                ],
              ],
            ),
          ),
        ),
        if (!canEdit)
          Padding(
            padding: const EdgeInsets.only(top: 10),
            child: Text('Changing roles needs role.manage.', style: TextStyle(color: c.muted, fontSize: 12.5)),
          ),
      ],
    );
  }
}

Future<void> showRoleEditor(BuildContext context, WidgetRef ref, {required List<PermissionGroup> catalog, RoleInfo? role, bool readOnly = false}) =>
    showDialog<void>(context: context, builder: (_) => _RoleEditor(catalog: catalog, role: role, readOnly: readOnly));

class _RoleEditor extends ConsumerStatefulWidget {
  const _RoleEditor({required this.catalog, this.role, this.readOnly = false});

  final List<PermissionGroup> catalog;
  final RoleInfo? role;
  final bool readOnly;

  @override
  ConsumerState<_RoleEditor> createState() => _RoleEditorState();
}

class _RoleEditorState extends ConsumerState<_RoleEditor> {
  late final _name = TextEditingController(text: widget.role?.name ?? '');
  late final _desc = TextEditingController(text: widget.role?.description ?? '');
  late final Set<String> _perms = {...?widget.role?.permissions};
  late RoleInfo? _role = widget.role;
  bool _busy = false;

  bool get _locked => widget.readOnly || (_role?.builtin ?? false);

  @override
  void dispose() {
    _name.dispose();
    _desc.dispose();
    super.dispose();
  }

  void _clone() {
    final r = _role!;
    setState(() {
      _role = RoleInfo(id: 0, name: '${r.name} (copy)', description: r.description, builtinKey: '', permissions: r.permissions, bindings: 0);
      _name.text = _role!.name;
      _desc.text = r.description;
    });
  }

  Future<void> _save() async {
    setState(() => _busy = true);
    final body = {'name': _name.text.trim(), 'description': _desc.text.trim(), 'permissions': _perms.toList()..sort()};
    try {
      final api = ref.read(apiProvider);
      final id = _role?.id ?? 0;
      if (id == 0) {
        await api.post('/roles', data: body);
      } else {
        await api.put('/roles/$id', data: body);
      }
      ref.invalidate(rolesProvider);
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _delete() async {
    final r = _role!;
    if (!await confirmAction(context, 'Delete role ${r.name}?', 'Roles still assigned to users cannot be deleted.', 'Delete', danger: true)) return;
    try {
      await ref.read(apiProvider).delete('/roles/${r.id}');
      ref.invalidate(rolesProvider);
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final role = _role;
    final isNew = role == null || role.id == 0;
    final canManage = ref.watch(sessionProvider).can('role.manage');
    return AlertDialog(
      title: Text(isNew ? (role == null ? 'New role' : 'New role from ${widget.role?.name}') : role!.name),
      content: SizedBox(
        width: 720,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (role?.builtin ?? false)
                Padding(
                  padding: const EdgeInsets.only(bottom: 10),
                  child: Text('Built-in roles cannot be changed. Clone it to make your own version.', style: TextStyle(color: c.muted)),
                ),
              if (!_locked) ...[
                TextField(controller: _name, decoration: const InputDecoration(labelText: 'Name')),
                const SizedBox(height: 10),
                TextField(controller: _desc, decoration: const InputDecoration(labelText: 'Description')),
                const SizedBox(height: 14),
              ],
              for (final g in widget.catalog) ...[
                Row(
                  children: [
                    Expanded(child: SectionLabel(g.name)),
                    if (!_locked)
                      TextButton(
                        onPressed: () => setState(() {
                          final keys = g.permissions.map((p) => p.key);
                          if (keys.every(_perms.contains)) {
                            _perms.removeAll(keys);
                          } else {
                            _perms.addAll(keys);
                          }
                        }),
                        child: const Text('all / none'),
                      ),
                  ],
                ),
                Wrap(
                  spacing: 4,
                  runSpacing: 0,
                  children: [
                    for (final p in g.permissions)
                      SizedBox(
                        width: 340,
                        child: CheckboxListTile(
                          dense: true,
                          contentPadding: EdgeInsets.zero,
                          controlAffinity: ListTileControlAffinity.leading,
                          value: _perms.contains(p.key),
                          onChanged: _locked ? null : (v) => setState(() => v == true ? _perms.add(p.key) : _perms.remove(p.key)),
                          title: Text(p.key, style: const TextStyle(fontFamily: 'monospace', fontSize: 12.5)),
                          subtitle: Text('${p.description}${p.projectScope ? '' : ' · global only'}', style: TextStyle(fontSize: 11.5, color: c.muted)),
                        ),
                      ),
                  ],
                ),
                const SizedBox(height: 6),
              ],
              Text('"Global only" permissions have no effect when the role is given for one project.', style: TextStyle(color: c.muted, fontSize: 12)),
            ],
          ),
        ),
      ),
      actions: [
        if (!isNew && !role!.builtin && canManage)
          TextButton(onPressed: _delete, style: TextButton.styleFrom(foregroundColor: c.failed), child: const Text('Delete')),
        if (!isNew && canManage) TextButton(onPressed: _clone, child: const Text('Clone')),
        TextButton(onPressed: () => Navigator.pop(context), child: Text(_locked ? 'Close' : 'Cancel')),
        if (!_locked) FilledButton(onPressed: _busy ? null : _save, child: const Text('Save')),
      ],
    );
  }
}

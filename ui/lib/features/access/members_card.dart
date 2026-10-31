import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';

/// Project page: who has access, with the roles given for this project (project.members edits them).
class MembersCard extends ConsumerWidget {
  const MembersCard({super.key, required this.projectId});

  final int projectId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = S.of(context);
    final members = ref.watch(projectMembersProvider(projectId));
    final canEdit = ref.watch(sessionProvider.select((s) => s.can('project.members', projectId: projectId)));
    final c = NectarColors.of(context);
    return NCard(
      title: l.acceMembers,
      trailing: canEdit
          ? IconButton(
              tooltip: l.acceAddMember,
              icon: const Icon(Icons.person_add_alt_1_rounded, size: 20),
              onPressed: () => showDialog<void>(context: context, builder: (_) => _MemberDialog(projectId: projectId)),
            )
          : null,
      child: AsyncBlock(
        value: members,
        loadingHeight: 60,
        onRetry: () => ref.invalidate(projectMembersProvider(projectId)),
        data: (list) => Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            for (final m in list)
              InkWell(
                onTap: canEdit && !m.isOwner ? () => showDialog<void>(context: context, builder: (_) => _MemberDialog(projectId: projectId, member: m)) : null,
                child: Padding(
                  padding: const EdgeInsets.symmetric(vertical: 6),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(m.name.isEmpty ? m.email : m.name, style: TextStyle(fontWeight: FontWeight.w600, color: m.disabled ? c.muted : c.text)),
                      Text(
                        m.isOwner
                            ? 'Owner'
                            : [
                                ...m.roles.map((r) => r.roleName),
                                ...m.globalRoles.map((r) => '${r.roleName} (everywhere)'),
                              ].join(', '),
                        style: TextStyle(fontSize: 12, color: c.muted),
                      ),
                    ],
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }
}

class _MemberDialog extends ConsumerStatefulWidget {
  const _MemberDialog({required this.projectId, this.member});

  final int projectId;
  final ProjectMember? member;

  @override
  ConsumerState<_MemberDialog> createState() => _MemberDialogState();
}

class _MemberDialogState extends ConsumerState<_MemberDialog> {
  int? _user;
  late final Set<int> _roles = {...?widget.member?.roles.map((r) => r.roleId)};
  bool _busy = false;

  Future<void> _save({bool remove = false}) async {
    final uid = widget.member?.userId ?? _user;
    if (uid == null) return;
    setState(() => _busy = true);
    try {
      final api = ref.read(apiProvider);
      if (remove) {
        await api.delete('/projects/${widget.projectId}/members/$uid');
      } else {
        await api.put('/projects/${widget.projectId}/members/$uid', data: {'role_ids': _roles.toList()});
      }
      ref.invalidate(projectMembersProvider(widget.projectId));
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final roles = ref.watch(rolesProvider);
    final m = widget.member;
    final directory = m == null ? ref.watch(userDirectoryProvider) : null;
    return AlertDialog(
      title: Text(m == null ? 'Add member' : (m.name.isEmpty ? m.email : m.name)),
      content: SizedBox(
        width: 460,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (directory != null)
                AsyncBlock(
                  value: directory,
                  loadingHeight: 50,
                  data: (users) => DropdownButtonFormField<int>(
                    initialValue: _user,
                    isExpanded: true,
                    decoration: InputDecoration(labelText: s.acceUser),
                    items: [for (final u in users) DropdownMenuItem(value: u.id, child: Text('${u.label} · ${u.email}', overflow: TextOverflow.ellipsis))],
                    onChanged: (v) => setState(() => _user = v),
                  ),
                ),
              if (m != null && m.globalRoles.isNotEmpty)
                Padding(
                  padding: const EdgeInsets.only(bottom: 8),
                  child: Text('Everywhere: ${m.globalRoles.map((r) => r.roleName).join(', ')} (Settings → Users)', style: TextStyle(color: c.muted, fontSize: 12.5)),
                ),
              const SizedBox(height: 8),
              const SectionLabel('Roles in this project'),
              AsyncBlock(
                value: roles,
                loadingHeight: 50,
                data: (list) => Column(
                  children: [
                    for (final r in list)
                      CheckboxListTile(
                        dense: true,
                        contentPadding: EdgeInsets.zero,
                        controlAffinity: ListTileControlAffinity.leading,
                        value: _roles.contains(r.id),
                        onChanged: (v) => setState(() => v == true ? _roles.add(r.id) : _roles.remove(r.id)),
                        title: Text(r.name),
                        subtitle: r.description.isEmpty ? null : Text(r.description, style: TextStyle(fontSize: 12, color: c.muted)),
                      ),
                  ],
                ),
              ),
              Text(s.acceYouCanOnlyGiveRoles, style: TextStyle(color: c.muted, fontSize: 12)),
            ],
          ),
        ),
      ),
      actions: [
        if (m != null && m.roles.isNotEmpty)
          TextButton(
            onPressed: _busy ? null : () => _save(remove: true),
            style: TextButton.styleFrom(foregroundColor: c.failed),
            child: Text(s.acceRemoveFromProject),
          ),
        TextButton(onPressed: () => Navigator.pop(context), child: Text(s.actionCancel)),
        FilledButton(onPressed: _busy || (m == null && _user == null) ? null : _save, child: Text(s.actionSave)),
      ],
    );
  }
}

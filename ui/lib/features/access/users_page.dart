import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/hex.dart';

/// Settings → Users: invite people, give them roles (everywhere or per project), disable, reset 2FA.
class UsersPage extends ConsumerWidget {
  const UsersPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final session = ref.watch(sessionProvider);
    if (!session.can('user.manage')) {
      return const PageScaffold(
        title: 'Users',
        children: [NCard(child: EmptyState(icon: Icons.lock_outline_rounded, title: 'No access', message: 'Your role does not include user.manage.'))],
      );
    }
    final users = ref.watch(usersProvider);
    return PageScaffold(
      title: 'Users',
      subtitle: 'Who can sign in, and what they may do. Roles apply everywhere or to one project.',
      maxWidth: 980,
      onRefresh: () async => ref.invalidate(usersProvider),
      actions: [
        FilledButton.icon(
          onPressed: () => showDialog<void>(context: context, builder: (_) => const _NewUserDialog()),
          icon: const Icon(Icons.person_add_alt_1_rounded, size: 18),
          label: const Text('Add user'),
        ),
      ],
      children: [
        NCard(
          padding: const EdgeInsets.symmetric(vertical: 6),
          child: AsyncBlock(
            value: users,
            onRetry: () => ref.invalidate(usersProvider),
            data: (list) => Column(
              children: [
                for (var i = 0; i < list.length; i++) ...[
                  if (i > 0) const Divider(height: 1),
                  _UserRow(user: list[i], self: list[i].id == session.user?.id),
                ],
              ],
            ),
          ),
        ),
      ],
    );
  }
}

class _UserRow extends ConsumerWidget {
  const _UserRow({required this.user, required this.self});

  final AdminUser user;
  final bool self;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final c = NectarColors.of(context);
    final bindings = ref.watch(userBindingsProvider(user.id));
    final projects = ref.watch(projectsProvider);
    String scope(int? pid) {
      if (pid == null) return 'everywhere';
      final p = projects.value?.where((p) => p.id == pid).firstOrNull;
      return p?.name ?? 'project $pid';
    }

    final roles = user.isOwner
        ? 'Owner — everything'
        : bindings.when(
            data: (bs) => bs.isEmpty ? 'no roles: can only sign in' : bs.map((b) => '${b.roleName} (${scope(b.projectId)})').join(', '),
            loading: () => '…',
            error: (e, _) => describeError(e),
          );
    return ListTile(
      leading: HexBadge(
        size: 38,
        child: Text(
          (user.name.isEmpty ? user.email : user.name).characters.first.toUpperCase(),
          style: const TextStyle(fontWeight: FontWeight.w700, color: NectarTokens.onAmber),
        ),
      ),
      title: Wrap(
        spacing: 8,
        crossAxisAlignment: WrapCrossAlignment.center,
        children: [
          Text(user.label, style: const TextStyle(fontWeight: FontWeight.w600)),
          if (self) _Chip('you', c.muted),
          if (user.isOwner) _Chip('owner', NectarTokens.amber),
          if (user.disabled) _Chip('disabled', c.failed),
          if (user.mfaEnabled) _Chip('2FA', c.running),
        ],
      ),
      subtitle: Text('${user.email} · $roles', maxLines: 2, overflow: TextOverflow.ellipsis, style: TextStyle(fontSize: 12.5, color: c.muted)),
      trailing: const Icon(Icons.chevron_right_rounded),
      onTap: () => showDialog<void>(context: context, builder: (_) => _EditUserDialog(user: user, self: self)),
    );
  }
}

class _Chip extends StatelessWidget {
  const _Chip(this.text, this.color);

  final String text;
  final Color color;

  @override
  Widget build(BuildContext context) => Container(
        padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 1),
        decoration: BoxDecoration(color: color.withValues(alpha: 0.14), borderRadius: BorderRadius.circular(999)),
        child: Text(text, style: TextStyle(fontSize: 11, color: color, fontWeight: FontWeight.w600)),
      );
}

// ----------------------------------------------------------------- new user --

class _NewUserDialog extends ConsumerStatefulWidget {
  const _NewUserDialog();

  @override
  ConsumerState<_NewUserDialog> createState() => _NewUserDialogState();
}

class _NewUserDialogState extends ConsumerState<_NewUserDialog> {
  final _email = TextEditingController();
  final _name = TextEditingController();
  final _password = TextEditingController();
  bool _invite = true;
  bool _busy = false;
  String? _link;

  @override
  void dispose() {
    _email.dispose();
    _name.dispose();
    _password.dispose();
    super.dispose();
  }

  Future<void> _create() async {
    setState(() => _busy = true);
    try {
      final res = await ref.read(apiProvider).post('/users', data: {
        'email': _email.text.trim(),
        'name': _name.text.trim(),
        if (_invite) 'invite': true else 'password': _password.text,
      });
      ref.invalidate(usersProvider);
      final m = res is Map ? res : const {};
      if (!mounted) return;
      if (m['invite_url'] != null) {
        setState(() => _link = '${m['invite_url']}');
      } else {
        Navigator.pop(context);
        showSnack(context, m['emailed'] == true ? 'Invitation sent to ${_email.text.trim()}' : 'User created');
      }
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    if (_link != null) {
      return AlertDialog(
        title: const Text('Share the invitation link'),
        content: SizedBox(
          width: 520,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const Text('The panel cannot send emails (Settings → Email). Send this link yourself; it works for 7 days.'),
              const SizedBox(height: 10),
              SelectableText(_link!, style: const TextStyle(fontFamily: 'monospace', fontSize: 12.5)),
            ],
          ),
        ),
        actions: [
          TextButton(
            onPressed: () {
              Clipboard.setData(ClipboardData(text: _link!));
              showSnack(context, 'Copied');
            },
            child: const Text('Copy'),
          ),
          FilledButton(onPressed: () => Navigator.pop(context), child: const Text('Done')),
        ],
      );
    }
    return AlertDialog(
      title: const Text('Add user'),
      content: SizedBox(
        width: 460,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TextField(controller: _email, decoration: const InputDecoration(labelText: 'Email'), keyboardType: TextInputType.emailAddress),
            const SizedBox(height: 10),
            TextField(controller: _name, decoration: const InputDecoration(labelText: 'Name')),
            const SizedBox(height: 10),
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              value: _invite,
              onChanged: (v) => setState(() => _invite = v),
              title: const Text('Send an invitation'),
              subtitle: Text('They choose their own password', style: TextStyle(color: c.muted, fontSize: 12.5)),
            ),
            if (!_invite) TextField(controller: _password, obscureText: true, decoration: const InputDecoration(labelText: 'Password (10+ characters)')),
            const SizedBox(height: 8),
            Text('New users have no roles: give them one after creating them.', style: TextStyle(color: c.muted, fontSize: 12.5)),
          ],
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
        FilledButton(onPressed: _busy ? null : _create, child: Text(_busy ? 'Creating…' : 'Create')),
      ],
    );
  }
}

// ---------------------------------------------------------------- edit user --

class _EditUserDialog extends ConsumerStatefulWidget {
  const _EditUserDialog({required this.user, required this.self});

  final AdminUser user;
  final bool self;

  @override
  ConsumerState<_EditUserDialog> createState() => _EditUserDialogState();
}

class _EditUserDialogState extends ConsumerState<_EditUserDialog> {
  late final _name = TextEditingController(text: widget.user.name);
  late bool _disabled = widget.user.disabled;
  List<RoleBindingInfo>? _bindings;
  bool _busy = false;

  @override
  void dispose() {
    _name.dispose();
    super.dispose();
  }

  Future<void> _call(Future<void> Function() fn, String done) async {
    setState(() => _busy = true);
    try {
      await fn();
      ref.invalidate(usersProvider);
      ref.invalidate(userBindingsProvider(widget.user.id));
      if (mounted) showSnack(context, done);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _save() async {
    final api = ref.read(apiProvider);
    await _call(() async {
      await api.patch('/users/${widget.user.id}', data: {'name': _name.text.trim(), if (!widget.self) 'disabled': _disabled});
      if (_bindings != null) {
        await api.put('/users/${widget.user.id}/bindings', data: [for (final b in _bindings!) b.toJson()]);
      }
      if (mounted) Navigator.pop(context);
    }, 'Saved ${widget.user.label}');
  }

  Future<void> _delete() async {
    if (!await confirmAction(context, 'Delete ${widget.user.label}?', 'They can no longer sign in; their API tokens stop working. The audit log keeps their actions.', 'Delete',
        danger: true)) {
      return;
    }
    await _call(() async {
      await ref.read(apiProvider).delete('/users/${widget.user.id}');
      if (mounted) Navigator.pop(context);
    }, 'Deleted');
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final session = ref.watch(sessionProvider);
    final u = widget.user;
    final current = ref.watch(userBindingsProvider(u.id));
    final roles = ref.watch(rolesProvider);
    final projects = ref.watch(projectsProvider);
    if (_bindings == null && current.hasValue) _bindings = List.of(current.value!);
    final bindings = _bindings;
    return AlertDialog(
      title: Text(u.label),
      content: SizedBox(
        width: 600,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(u.email, style: TextStyle(color: c.muted)),
              const SizedBox(height: 12),
              TextField(controller: _name, decoration: const InputDecoration(labelText: 'Name')),
              if (!widget.self)
                SwitchListTile(
                  contentPadding: EdgeInsets.zero,
                  value: _disabled,
                  onChanged: (v) => setState(() => _disabled = v),
                  title: const Text('Disabled'),
                  subtitle: Text('Cannot sign in; sessions and tokens stop working', style: TextStyle(color: c.muted, fontSize: 12.5)),
                ),
              const SizedBox(height: 10),
              const SectionLabel('Roles'),
              if (u.isOwner)
                Text('Owners can do everything; roles do not change that.', style: TextStyle(color: c.muted))
              else if (bindings == null || !roles.hasValue || !projects.hasValue)
                const LinearProgressIndicator()
              else ...[
                for (var i = 0; i < bindings.length; i++)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 8),
                    child: Row(
                      children: [
                        Expanded(
                          child: DropdownButtonFormField<int>(
                            key: ValueKey('role-$i-${bindings[i].roleId}'),
                            initialValue: roles.value!.any((r) => r.id == bindings[i].roleId) ? bindings[i].roleId : null,
                            isExpanded: true,
                            decoration: const InputDecoration(labelText: 'Role', isDense: true),
                            items: [for (final r in roles.value!) DropdownMenuItem(value: r.id, child: Text(r.name))],
                            onChanged: (v) => setState(() => bindings[i] = RoleBindingInfo(roleId: v ?? bindings[i].roleId, roleName: '', projectId: bindings[i].projectId)),
                          ),
                        ),
                        const SizedBox(width: 10),
                        Expanded(
                          child: DropdownButtonFormField<int>(
                            key: ValueKey('scope-$i-${bindings[i].projectId}'),
                            initialValue: bindings[i].projectId ?? 0,
                            isExpanded: true,
                            decoration: const InputDecoration(labelText: 'Where', isDense: true),
                            items: [
                              const DropdownMenuItem(value: 0, child: Text('Everywhere')),
                              for (final p in projects.value!) DropdownMenuItem(value: p.id, child: Text(p.name)),
                            ],
                            onChanged: (v) => setState(
                                () => bindings[i] = RoleBindingInfo(roleId: bindings[i].roleId, roleName: '', projectId: v == null || v == 0 ? null : v)),
                          ),
                        ),
                        IconButton(tooltip: 'Remove', icon: const Icon(Icons.close_rounded, size: 18), onPressed: () => setState(() => bindings.removeAt(i))),
                      ],
                    ),
                  ),
                Align(
                  alignment: Alignment.centerLeft,
                  child: TextButton.icon(
                    onPressed: roles.value!.isEmpty
                        ? null
                        : () => setState(() => bindings.add(RoleBindingInfo(
                            roleId: (roles.value!.where((r) => r.builtinKey == 'viewer').firstOrNull ?? roles.value!.first).id, roleName: ''))),
                    icon: const Icon(Icons.add_rounded, size: 18),
                    label: const Text('Add role'),
                  ),
                ),
                Text('You can only give or take away roles whose permissions you hold yourself.', style: TextStyle(color: c.muted, fontSize: 12.5)),
              ],
              const SizedBox(height: 14),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  if (u.mfaEnabled && !widget.self)
                    OutlinedButton.icon(
                      onPressed: _busy
                          ? null
                          : () => _call(() => ref.read(apiProvider).delete('/users/${u.id}/mfa').then((_) {}), 'Two-factor authentication turned off for ${u.label}'),
                      icon: const Icon(Icons.phonelink_erase_rounded, size: 18),
                      label: const Text('Reset 2FA'),
                    ),
                  if (!widget.self)
                    OutlinedButton.icon(
                      onPressed: _busy
                          ? null
                          : () => _call(() async {
                                final res = await ref.read(apiProvider).post('/users/${u.id}/invite');
                                final link = res is Map ? res['invite_url'] : null;
                                if (link != null) await Clipboard.setData(ClipboardData(text: '$link'));
                              }, 'Invitation sent (or its link copied when email is off)'),
                      icon: const Icon(Icons.forward_to_inbox_rounded, size: 18),
                      label: const Text('Send invitation / reset link'),
                    ),
                  if (!widget.self && (!u.isOwner || session.permissions.owner))
                    OutlinedButton.icon(
                      style: OutlinedButton.styleFrom(foregroundColor: c.failed),
                      onPressed: _busy ? null : _delete,
                      icon: const Icon(Icons.delete_outline_rounded, size: 18),
                      label: const Text('Delete'),
                    ),
                ],
              ),
              if (u.createdAt != null) ...[
                const SizedBox(height: 10),
                Text('Added ${timeAgo(u.createdAt)}', style: TextStyle(color: c.muted, fontSize: 12)),
              ],
            ],
          ),
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
        FilledButton(onPressed: _busy ? null : _save, child: const Text('Save')),
      ],
    );
  }
}

import 'package:flutter/material.dart';

import '../../widgets/hex.dart';

class SplashPage extends StatelessWidget {
  const SplashPage({super.key});

  @override
  Widget build(BuildContext context) => const Scaffold(body: Center(child: HexLoader(size: 46)));
}

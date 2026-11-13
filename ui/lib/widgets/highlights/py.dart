import 'package:material_ui/material_ui.dart';
import 'package:nectar_apps_ui/widgets/highlights/syntax_rule.dart';

final pythonSyntaxRules = <SyntaxRule>[
  // Comments
  SyntaxRule(
    RegExp(r'#[^\n]*'),
    const TextStyle(color: Color(0xFF7F848E), fontStyle: FontStyle.italic),
  ),

  // Triple-quoted strings and docstrings
  SyntaxRule(
    RegExp(
      r"""(?:'''[\s\S]*?'''|"""
      r'"""[\s\S]*?"""'
      r')',
    ),
    const TextStyle(color: Color(0xFF98C379)),
  ),

  // Regular strings and f-strings
  SyntaxRule(
    RegExp(r"""(?i:[rubf]{0,2})(?:"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*')"""),
    const TextStyle(color: Color(0xFF98C379)),
  ),

  // Decorators
  SyntaxRule(
    RegExp(r'@[a-zA-Z_][\w.]*'),
    const TextStyle(color: Color(0xFFE5C07B)),
  ),

  // Function and class declarations
  SyntaxRule(
    RegExp(r'\b(?:def|class)\s+[a-zA-Z_]\w*'),
    const TextStyle(color: Color(0xFFE5C07B), fontWeight: FontWeight.bold),
  ),

  // Keywords
  SyntaxRule(
    RegExp(
      r'\b(and|as|assert|async|await|break|case|class|continue|'
      r'def|del|elif|else|except|finally|for|from|global|if|'
      r'import|in|is|lambda|match|nonlocal|not|or|pass|raise|'
      r'return|try|while|with|yield)\b',
    ),
    const TextStyle(color: Color(0xFFC678DD), fontWeight: FontWeight.bold),
  ),

  // Boolean and None
  SyntaxRule(
    RegExp(r'\b(True|False|None|NotImplemented|Ellipsis)\b'),
    const TextStyle(color: Color(0xFFD19A66)),
  ),

  // Built-in functions
  SyntaxRule(
    RegExp(
      r'\b(print|len|range|enumerate|zip|map|filter|sorted|'
      r'sum|min|max|abs|round|all|any|open|input|type|'
      r'isinstance|issubclass|super|property|classmethod|'
      r'staticmethod|repr|str|int|float|bool|list|tuple|'
      r'dict|set|frozenset|bytes|bytearray|object|'
      r'hasattr|getattr|setattr|delattr|vars|dir|help|'
      r'iter|next|reversed|format|hash|id|callable)\b',
    ),
    const TextStyle(color: Color(0xFF61AFEF)),
  ),

  // Special variables
  SyntaxRule(RegExp(r'\b__\w+__\b'), const TextStyle(color: Color(0xFFE5C07B))),

  // Numbers
  SyntaxRule(
    RegExp(
      r'\b(?:0[xX][0-9a-fA-F_]+|0[bB][01_]+|0[oO][0-7_]+|'
      r'\d[\d_]*(?:\.[\d_]*)?(?:[eE][+-]?\d[\d_]*)?j?)\b',
    ),
    const TextStyle(color: Color(0xFFD19A66)),
  ),

  // Operators
  SyntaxRule(
    RegExp(
      r'\*\*=|//=|>>=|<<=|:=|==|!=|<=|>=|'
      r'\*\*|//|<<|>>|->|\+=|-=|\*=|/=|%=|'
      r'&&|\|\||[+\-*/%=<>!&|^~@]',
    ),
    const TextStyle(color: Color(0xFFE06C75)),
  ),

  // Punctuation
  SyntaxRule(
    RegExp(r'[{}()\[\].,:;]'),
    const TextStyle(color: Color(0xFFABB2BF)),
  ),
];

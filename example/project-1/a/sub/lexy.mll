rule lexy = parse
  | (['a'-'z'] as c) eof
  | {c}

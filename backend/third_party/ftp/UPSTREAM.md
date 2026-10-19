# FTP source

Based on github.com/jlaffaye/ftp v0.2.4. The upstream license is retained in LICENSE.

Local changes add bounded, cancellable directory metadata operations and recognize MLSD symbolic-link types. Ordinary transfer methods retain their upstream behavior.

Bounded legacy LIST scans ignore one leading decimal `total` summary and the first `.` and `..` entries. Repeated summaries and malformed lines remain errors and consume the scan budget. MLSD parsing does not accept LIST summaries.

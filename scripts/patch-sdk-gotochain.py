"""Patch GOTOOLCHAIN=local into SDK golang infrastructure for offline builds."""
import re
import sys

def patch_file(path, patches):
	"""patches is a list of (description, pattern, replacement) tuples applied in order.
	Skips if GOTOOLCHAIN=local already exists in file."""
	with open(path, 'r') as f:
		content = f.read()
	if 'GOTOOLCHAIN=local' in content:
		print(f'  {path}: already patched, skipping')
		return
	for desc, pattern, replacement in patches:
		content = re.sub(pattern, replacement, content)
	with open(path, 'w') as f:
		f.write(content)
	print(f'  {path}: patched')

def main():
	golang_dir = sys.argv[1]

	goenv_middle = (r'(\tGOENV=off)\s*\\\n', r'\1 \\\n\tGOTOOLCHAIN=local \\\n')
	goenv_end = (r'(\tGOENV=off)\n', r'\1 \\\n\tGOTOOLCHAIN=local\n')

	path_end = (r'(PATH="[^"]*openwrt:[^"]*")\n', r'\1 \\\n\tGOTOOLCHAIN=local\n')
	path_middle = (r'(PATH="[^"]*openwrt:[^"]*")\s*\\\n', r'\1 \\\n\tGOTOOLCHAIN=local \\\n')

	patch_file(
		f'{golang_dir}/golang-version.mk',
		[('GOENV middle', *goenv_middle), ('GOENV end', *goenv_end)],
	)

	with open(f'{golang_dir}/golang-host-build.mk', 'r') as f:
		content = f.read()
	if 'GOTOOLCHAIN=local' not in content:
		content = re.sub(*goenv_middle, content)
		content = re.sub(*goenv_end, content)
		content = re.sub(*path_middle, content)
		content = re.sub(*path_end, content)
		with open(f'{golang_dir}/golang-host-build.mk', 'w') as f:
			f.write(content)
		print(f'  {golang_dir}/golang-host-build.mk: patched')
	else:
		print(f'  {golang_dir}/golang-host-build.mk: already patched, skipping')

	patch_file(
		f'{golang_dir}/golang-bootstrap/Makefile',
		[('GOENV middle', *goenv_middle), ('GOENV end', *goenv_end)],
	)

if __name__ == '__main__':
	main()
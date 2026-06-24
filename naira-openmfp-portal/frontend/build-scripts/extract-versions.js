const fs = require('fs');
const path = require('path');

const packageJsonPath = path.join(__dirname, '..', 'package.json');
const outputPath = path.join(__dirname, '..', 'src', 'assets', 'dependencies-versions.json');

try {
  const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, 'utf8'));
  const versions = {
    ...packageJson.dependencies,
    ...packageJson.devDependencies,
  };
  
  fs.mkdirSync(path.dirname(outputPath), { recursive: true });
  fs.writeFileSync(outputPath, JSON.stringify(versions, null, 2));
  
  console.log('✓ Dependencies versions extracted');
} catch (error) {
  console.error('Error extracting versions:', error);
  process.exit(1);
}

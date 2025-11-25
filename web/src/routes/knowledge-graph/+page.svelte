<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>多维知识体系立方体 - Svelte</title>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/three.js/r128/three.min.js"></script>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
            background: linear-gradient(135deg, #0f0c29 0%, #302b63 50%, #24243e 100%);
            color: #fff;
            overflow: hidden;
            height: 100vh;
        }

        .container {
            display: flex;
            height: 100vh;
        }

        .sidebar {
            width: 320px;
            background: rgba(15, 12, 41, 0.95);
            padding: 20px;
            overflow-y: auto;
            border-right: 1px solid rgba(255, 255, 255, 0.1);
            backdrop-filter: blur(10px);
        }

        .canvas-container {
            flex: 1;
            position: relative;
        }

        .info-panel {
            position: absolute;
            top: 20px;
            right: 20px;
            width: 320px;
            background: rgba(15, 12, 41, 0.95);
            padding: 20px;
            border-radius: 12px;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255, 255, 255, 0.1);
            max-height: 80vh;
            overflow-y: auto;
        }

        h2 {
            font-size: 18px;
            margin-bottom: 15px;
            color: #64ffda;
            border-bottom: 2px solid rgba(100, 255, 218, 0.3);
            padding-bottom: 8px;
        }

        h3 {
            font-size: 16px;
            margin: 15px 0 10px;
            color: #8892ff;
        }

        .dimension-group {
            margin-bottom: 20px;
            padding: 15px;
            background: rgba(255, 255, 255, 0.05);
            border-radius: 8px;
            border: 1px solid rgba(255, 255, 255, 0.1);
        }

        .dimension-label {
            font-size: 14px;
            font-weight: 600;
            margin-bottom: 8px;
            color: #64ffda;
        }

        input, select, textarea {
            width: 100%;
            padding: 10px;
            margin-bottom: 10px;
            background: rgba(255, 255, 255, 0.1);
            border: 1px solid rgba(255, 255, 255, 0.2);
            border-radius: 6px;
            color: #fff;
            font-size: 13px;
            transition: all 0.3s;
        }

        input:focus, select:focus, textarea:focus {
            outline: none;
            border-color: #64ffda;
            background: rgba(255, 255, 255, 0.15);
        }

        button {
            width: 100%;
            padding: 12px;
            margin-top: 10px;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            border: none;
            border-radius: 6px;
            color: #fff;
            font-size: 14px;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.3s;
        }

        button:hover {
            transform: translateY(-2px);
            box-shadow: 0 8px 20px rgba(102, 126, 234, 0.4);
        }

        button:active {
            transform: translateY(0);
        }

        .btn-secondary {
            background: linear-gradient(135deg, #f093fb 0%, #f5576c 100%);
        }

        .btn-success {
            background: linear-gradient(135deg, #4facfe 0%, #00f2fe 100%);
        }

        .control-group {
            margin-bottom: 20px;
            padding: 15px;
            background: rgba(255, 255, 255, 0.05);
            border-radius: 8px;
        }

        .slider-container {
            margin: 15px 0;
        }

        .slider-label {
            display: flex;
            justify-content: space-between;
            margin-bottom: 8px;
            font-size: 13px;
            color: #b0b8ff;
        }

        input[type="range"] {
            width: 100%;
            height: 6px;
            background: rgba(255, 255, 255, 0.1);
            border-radius: 3px;
            outline: none;
        }

        input[type="range"]::-webkit-slider-thumb {
            width: 16px;
            height: 16px;
            background: #64ffda;
            cursor: pointer;
            border-radius: 50%;
        }

        .node-item {
            padding: 12px;
            margin: 8px 0;
            background: rgba(255, 255, 255, 0.08);
            border-radius: 6px;
            border-left: 3px solid #64ffda;
            cursor: pointer;
            transition: all 0.3s;
        }

        .node-item:hover {
            background: rgba(100, 255, 218, 0.15);
            transform: translateX(5px);
        }

        .node-title {
            font-weight: 600;
            margin-bottom: 5px;
            color: #fff;
        }

        .node-desc {
            font-size: 12px;
            color: #b0b8ff;
            line-height: 1.4;
        }

        .tag {
            display: inline-block;
            padding: 4px 10px;
            margin: 4px 4px 4px 0;
            background: rgba(100, 255, 218, 0.2);
            border-radius: 12px;
            font-size: 11px;
            color: #64ffda;
        }

        .controls-toolbar {
            position: absolute;
            top: 20px;
            left: 20px;
            background: rgba(15, 12, 41, 0.95);
            padding: 15px;
            border-radius: 12px;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255, 255, 255, 0.1);
        }

        .toolbar-btn {
            width: auto;
            padding: 8px 16px;
            margin: 5px;
            display: inline-block;
            font-size: 12px;
        }

        ::-webkit-scrollbar {
            width: 8px;
        }

        ::-webkit-scrollbar-track {
            background: rgba(255, 255, 255, 0.05);
        }

        ::-webkit-scrollbar-thumb {
            background: rgba(100, 255, 218, 0.3);
            border-radius: 4px;
        }

        ::-webkit-scrollbar-thumb:hover {
            background: rgba(100, 255, 218, 0.5);
        }
    </style>
</head>
<body>
    <div id="app"></div>

    <script type="module">
        import { writable, derived } from 'https://cdn.jsdelivr.net/npm/svelte@3/store/index.mjs';

        // Create Svelte stores
        const dimensions = writable({
            x: { name: '生命周期阶段', levels: ['规划', '设计', '实施', '运维'] },
            y: { name: '架构层次', levels: ['资源层', '平台层', '服务层', '应用层'] },
            z: { name: '业务领域', levels: ['业务架构', '数据架构', '应用架构', '技术架构'] }
        });

        const nodes = writable([]);
        const selectedNode = writable(null);
        const autoRotate = writable(false);

        const newNode = writable({
            title: '',
            description: '',
            tags: '',
            position: { x: 2, y: 2, z: 2 }
        });

        // Svelte component
        function createApp(target) {
            let scene, camera, renderer, cube, nodeObjects = [];
            let mouseDown = false, mouseX = 0, mouseY = 0;
            let targetRotationX = 0, targetRotationY = 0;
            let canvasContainer;

            // Subscribe to stores
            let currentDimensions, currentNodes, currentAutoRotate;
            dimensions.subscribe(v => currentDimensions = v);
            nodes.subscribe(v => currentNodes = v);
            autoRotate.subscribe(v => currentAutoRotate = v);

            // Initialize sample nodes
            nodes.set([
                { 
                    id: 1,
                    title: '微服务架构设计', 
                    position: { x: 1, y: 2, z: 2 }, 
                    description: '基于微服务的架构设计原则和最佳实践', 
                    tags: ['架构', '微服务', '设计'] 
                },
                { 
                    id: 2,
                    title: '容器编排平台', 
                    position: { x: 2, y: 1, z: 3 }, 
                    description: 'Kubernetes等容器编排技术的应用', 
                    tags: ['容器', 'K8s', '运维'] 
                },
                { 
                    id: 3,
                    title: '数据治理体系', 
                    position: { x: 0, y: 3, z: 1 }, 
                    description: '企业级数据治理框架与实施路径', 
                    tags: ['数据', '治理', '标准'] 
                }
            ]);

            function setupScene() {
                scene = new THREE.Scene();
                scene.background = new THREE.Color(0x0f0c29);

                camera = new THREE.PerspectiveCamera(60, 1, 0.1, 1000);
                camera.position.set(8, 8, 8);
                camera.lookAt(0, 0, 0);

                renderer = new THREE.WebGLRenderer({ antialias: true });
                
                const ambientLight = new THREE.AmbientLight(0xffffff, 0.6);
                scene.add(ambientLight);

                const directionalLight = new THREE.DirectionalLight(0xffffff, 0.8);
                directionalLight.position.set(10, 10, 10);
                scene.add(directionalLight);
            }

            function createCube() {
                const cubeGroup = new THREE.Group();
                const gridSize = 4;
                const spacing = 2;
                const lineMaterial = new THREE.LineBasicMaterial({ 
                    color: 0x64ffda, 
                    opacity: 0.3, 
                    transparent: true 
                });

                // Create grid lines
                for (let i = 0; i < gridSize; i++) {
                    for (let j = 0; j < gridSize; j++) {
                        const points = [];
                        for (let k = 0; k < gridSize; k++) {
                            points.push(new THREE.Vector3(i * spacing - 3, j * spacing - 3, k * spacing - 3));
                        }
                        const geometry = new THREE.BufferGeometry().setFromPoints(points);
                        const line = new THREE.Line(geometry, lineMaterial);
                        cubeGroup.add(line);
                    }
                }

                for (let i = 0; i < gridSize; i++) {
                    for (let k = 0; k < gridSize; k++) {
                        const points = [];
                        for (let j = 0; j < gridSize; j++) {
                            points.push(new THREE.Vector3(i * spacing - 3, j * spacing - 3, k * spacing - 3));
                        }
                        const geometry = new THREE.BufferGeometry().setFromPoints(points);
                        const line = new THREE.Line(geometry, lineMaterial);
                        cubeGroup.add(line);
                    }
                }

                for (let j = 0; j < gridSize; j++) {
                    for (let k = 0; k < gridSize; k++) {
                        const points = [];
                        for (let i = 0; i < gridSize; i++) {
                            points.push(new THREE.Vector3(i * spacing - 3, j * spacing - 3, k * spacing - 3));
                        }
                        const geometry = new THREE.BufferGeometry().setFromPoints(points);
                        const line = new THREE.Line(geometry, lineMaterial);
                        cubeGroup.add(line);
                    }
                }

                cube = cubeGroup;
                scene.add(cubeGroup);
            }

            function createAxes() {
                const axisLength = 8;
                const axisGroup = new THREE.Group();

                const xAxis = new THREE.ArrowHelper(
                    new THREE.Vector3(1, 0, 0),
                    new THREE.Vector3(-4, -4, -4),
                    axisLength, 0xff6b6b, 0.5, 0.3
                );
                axisGroup.add(xAxis);

                const yAxis = new THREE.ArrowHelper(
                    new THREE.Vector3(0, 1, 0),
                    new THREE.Vector3(-4, -4, -4),
                    axisLength, 0x51cf66, 0.5, 0.3
                );
                axisGroup.add(yAxis);

                const zAxis = new THREE.ArrowHelper(
                    new THREE.Vector3(0, 0, 1),
                    new THREE.Vector3(-4, -4, -4),
                    axisLength, 0x4dabf7, 0.5, 0.3
                );
                axisGroup.add(zAxis);

                scene.add(axisGroup);
            }

            function createNodeMesh(nodeData) {
                const geometry = new THREE.SphereGeometry(0.3, 16, 16);
                const material = new THREE.MeshPhongMaterial({
                    color: 0x64ffda,
                    emissive: 0x64ffda,
                    emissiveIntensity: 0.3,
                    transparent: true,
                    opacity: 0.9
                });
                
                const sphere = new THREE.Mesh(geometry, material);
                const pos = nodeData.position;
                sphere.position.set(pos.x * 2 - 3, pos.y * 2 - 3, pos.z * 2 - 3);
                sphere.userData = nodeData;

                const glowGeometry = new THREE.SphereGeometry(0.4, 16, 16);
                const glowMaterial = new THREE.MeshBasicMaterial({
                    color: 0x64ffda,
                    transparent: true,
                    opacity: 0.2
                });
                const glow = new THREE.Mesh(glowGeometry, glowMaterial);
                sphere.add(glow);

                return sphere;
            }

            function updateNodes() {
                nodeObjects.forEach(obj => scene.remove(obj));
                nodeObjects = [];
                
                currentNodes.forEach(nodeData => {
                    const mesh = createNodeMesh(nodeData);
                    nodeObjects.push(mesh);
                    scene.add(mesh);
                });
            }

            function setupControls() {
                const canvas = renderer.domElement;

                canvas.addEventListener('mousedown', (e) => {
                    mouseDown = true;
                    mouseX = e.clientX;
                    mouseY = e.clientY;
                });

                canvas.addEventListener('mouseup', () => {
                    mouseDown = false;
                });

                canvas.addEventListener('mousemove', (e) => {
                    if (mouseDown) {
                        const deltaX = e.clientX - mouseX;
                        const deltaY = e.clientY - mouseY;
                        
                        targetRotationY += deltaX * 0.01;
                        targetRotationX += deltaY * 0.01;
                        
                        mouseX = e.clientX;
                        mouseY = e.clientY;
                    }
                });

                canvas.addEventListener('click', (e) => {
                    const mouse = new THREE.Vector2();
                    const rect = canvas.getBoundingClientRect();
                    mouse.x = ((e.clientX - rect.left) / rect.width) * 2 - 1;
                    mouse.y = -((e.clientY - rect.top) / rect.height) * 2 + 1;

                    const raycaster = new THREE.Raycaster();
                    raycaster.setFromCamera(mouse, camera);
                    
                    const intersects = raycaster.intersectObjects(nodeObjects);
                    if (intersects.length > 0) {
                        selectedNode.set(intersects[0].object.userData);
                    }
                });

                canvas.addEventListener('wheel', (e) => {
                    e.preventDefault();
                    const delta = e.deltaY * 0.01;
                    camera.position.multiplyScalar(1 + delta * 0.1);
                });
            }

            function animate() {
                requestAnimationFrame(animate);

                if (currentAutoRotate) {
                    targetRotationY += 0.005;
                }

                cube.rotation.y += (targetRotationY - cube.rotation.y) * 0.1;
                cube.rotation.x += (targetRotationX - cube.rotation.x) * 0.1;

                nodeObjects.forEach((node, index) => {
                    const time = Date.now() * 0.001;
                    if (node.children[0]) {
                        node.children[0].rotation.y = time + index;
                        node.children[0].scale.set(
                            1 + Math.sin(time * 2 + index) * 0.1,
                            1 + Math.sin(time * 2 + index) * 0.1,
                            1 + Math.sin(time * 2 + index) * 0.1
                        );
                    }
                });

                renderer.render(scene, camera);
            }

            function handleResize() {
                if (canvasContainer) {
                    camera.aspect = canvasContainer.clientWidth / canvasContainer.clientHeight;
                    camera.updateProjectionMatrix();
                    renderer.setSize(canvasContainer.clientWidth, canvasContainer.clientHeight);
                }
            }

            // Component template
            target.innerHTML = `
                <div class="container">
                    <div class="sidebar">
                        <h2>🎯 维度配置</h2>
                        
                        <div class="dimension-group">
                            <div class="dimension-label">X 轴维度</div>
                            <input type="text" id="x-name" placeholder="维度名称">
                            <input type="text" id="x-levels" placeholder="层级 (逗号分隔)">
                        </div>

                        <div class="dimension-group">
                            <div class="dimension-label">Y 轴维度</div>
                            <input type="text" id="y-name" placeholder="维度名称">
                            <input type="text" id="y-levels" placeholder="层级 (逗号分隔)">
                        </div>

                        <div class="dimension-group">
                            <div class="dimension-label">Z 轴维度</div>
                            <input type="text" id="z-name" placeholder="维度名称">
                            <input type="text" id="z-levels" placeholder="层级 (逗号分隔)">
                        </div>

                        <button id="update-dims">🔄 更新维度</button>

                        <h2 style="margin-top: 30px;">📦 添加知识节点</h2>
                        
                        <div class="control-group">
                            <input type="text" id="node-title" placeholder="节点标题">
                            <textarea id="node-desc" placeholder="节点描述" style="height: 60px; resize: vertical;"></textarea>
                            <input type="text" id="node-tags" placeholder="标签 (逗号分隔)">
                            
                            <div class="slider-container">
                                <div class="slider-label">
                                    <span>X 位置</span>
                                    <span id="x-pos-val">2</span>
                                </div>
                                <input type="range" id="node-x" min="0" max="3" value="2">
                            </div>

                            <div class="slider-container">
                                <div class="slider-label">
                                    <span>Y 位置</span>
                                    <span id="y-pos-val">2</span>
                                </div>
                                <input type="range" id="node-y" min="0" max="3" value="2">
                            </div>

                            <div class="slider-container">
                                <div class="slider-label">
                                    <span>Z 位置</span>
                                    <span id="z-pos-val">2</span>
                                </div>
                                <input type="range" id="node-z" min="0" max="3" value="2">
                            </div>

                            <button class="btn-success" id="add-node">➕ 添加节点</button>
                        </div>

                        <h2 style="margin-top: 20px;">📋 知识节点列表</h2>
                        <div id="nodes-list"></div>
                    </div>

                    <div class="canvas-container" id="canvas-container">
                        <div class="controls-toolbar">
                            <button class="toolbar-btn" id="reset-view">🔄 重置视角</button>
                            <button class="toolbar-btn btn-secondary" id="toggle-rotate">🔄 自动旋转</button>
                        </div>
                        <div class="info-panel" id="info-panel" style="display: none;">
                            <h2>ℹ️ 节点详情</h2>
                            <div id="node-details"></div>
                        </div>
                    </div>
                </div>
            `;

            // Initialize Three.js
            setupScene();
            createCube();
            createAxes();

            // Mount canvas
            canvasContainer = target.querySelector('#canvas-container');
            canvasContainer.insertBefore(renderer.domElement, canvasContainer.firstChild);
            handleResize();
            
            setupControls();
            updateNodes();
            animate();

            window.addEventListener('resize', handleResize);

            // Bind input values
            const xNameInput = target.querySelector('#x-name');
            const yNameInput = target.querySelector('#y-name');
            const zNameInput = target.querySelector('#z-name');
            const xLevelsInput = target.querySelector('#x-levels');
            const yLevelsInput = target.querySelector('#y-levels');
            const zLevelsInput = target.querySelector('#z-levels');

            dimensions.subscribe(dims => {
                xNameInput.value = dims.x.name;
                yNameInput.value = dims.y.name;
                zNameInput.value = dims.z.name;
                xLevelsInput.value = dims.x.levels.join(',');
                yLevelsInput.value = dims.y.levels.join(',');
                zLevelsInput.value = dims.z.levels.join(',');
            });

            // Update dimensions
            target.querySelector('#update-dims').addEventListener('click', () => {
                dimensions.update(dims => ({
                    x: { 
                        name: xNameInput.value, 
                        levels: xLevelsInput.value.split(',').map(l => l.trim()) 
                    },
                    y: { 
                        name: yNameInput.value, 
                        levels: yLevelsInput.value.split(',').map(l => l.trim()) 
                    },
                    z: { 
                        name: zNameInput.value, 
                        levels: zLevelsInput.value.split(',').map(l => l.trim()) 
                    }
                }));
                alert('维度配置已更新!');
            });

            // Add node
            const nodeXInput = target.querySelector('#node-x');
            const nodeYInput = target.querySelector('#node-y');
            const nodeZInput = target.querySelector('#node-z');
            const xPosVal = target.querySelector('#x-pos-val');
            const yPosVal = target.querySelector('#y-pos-val');
            const zPosVal = target.querySelector('#z-pos-val');

            nodeXInput.addEventListener('input', (e) => xPosVal.textContent = e.target.value);
            nodeYInput.addEventListener('input', (e) => yPosVal.textContent = e.target.value);
            nodeZInput.addEventListener('input', (e) => zPosVal.textContent = e.target.value);

            target.querySelector('#add-node').addEventListener('click', () => {
                const title = target.querySelector('#node-title').value;
                const description = target.querySelector('#node-desc').value;
                const tags = target.querySelector('#node-tags').value.split(',').map(t => t.trim()).filter(t => t);

                if (!title) {
                    alert('请输入节点标题');
                    return;
                }

                nodes.update(n => [...n, {
                    id: Date.now(),
                    title,
                    description,
                    tags,
                    position: {
                        x: parseInt(nodeXInput.value),
                        y: parseInt(nodeYInput.value),
                        z: parseInt(nodeZInput.value)
                    }
                }]);

                target.querySelector('#node-title').value = '';
                target.querySelector('#node-desc').value = '';
                target.querySelector('#node-tags').value = '';
                
                updateNodes();
            });

            // Update nodes list
            const nodesList = target.querySelector('#nodes-list');
            nodes.subscribe(n => {
                nodesList.innerHTML = n.map(node => `
                    <div class="node-item" data-node-id="${node.id}">
                        <div class="node-title">${node.title}</div>
                        <div class="node-desc">${node.description || '暂无描述'}</div>
                        <div>
                            ${node.tags ? node.tags.map(tag => `<span class="tag">${tag}</span>`).join('') : ''}
                        </div>
                    </div>
                `).join('');

                // Add click handlers
                nodesList.querySelectorAll('.node-item').forEach(item => {
                    item.addEventListener('click', () => {
                        const id = parseInt(item.dataset.nodeId);
                        const node = n.find(nd => nd.id === id);
                        if (node) selectedNode.set(node);
                    });
                });
            });

            // Show node details
            const infoPanel = target.querySelector('#info-panel');
            const nodeDetails = target.querySelector('#node-details');
            selectedNode.subscribe(node => {
                if (node) {
                    nodeDetails.innerHTML = `
                        <h3>${node.title}</h3>
                        <p style="margin: 10px 0; line-height: 1.6;">${node.description || '暂无描述'}</p>
                        <div style="margin-top: 15px;">
                            <strong style="color: #8892ff;">位置:</strong><br>
                            X: ${node.position.x}, Y: ${node.position.y}, Z: ${node.position.z}
                        </div>
                        <div style="margin-top: 10px;">
                            ${node.tags ? node.tags.map(tag => `<span class="tag">${tag}</span>`).join('') : ''}
                        </div>
                    `;
                    infoPanel.style.display = 'block';
                } else {
                    infoPanel.style.display = 'none';
                }
            });

            // Controls
            target.querySelector('#reset-view').addEventListener('click', () => {
                camera.position.set(8, 8, 8);
                camera.lookAt(0, 0, 0);
                targetRotationX = 0;
                targetRotationY = 0;
            });

            target.querySelector('#toggle-rotate').addEventListener('click', () => {
                autoRotate.update(v => !v);
            });

            nodes.subscribe(() => updateNodes());

            return {
                destroy() {
                    window.removeEventListener('resize', handleResize);
                    renderer.dispose();
                }
            };
        }

        // Mount app
        createApp(document.getElementById('app'));
    </script>
</body>
</html>
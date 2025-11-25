<script lang="ts">
    import * as THREE from "three";

        // 数字化知识体系数据
        const knowledgeSystem = {
            dimensions: {
                x: { 
                    name: '生命周期阶段', 
                    levels: ['战略规划', '架构设计', '实施交付', '运营治理'] 
                },
                y: { 
                    name: '架构层次', 
                    levels: ['基础资源', '平台能力', '技术服务', '业务应用'] 
                },
                z: { 
                    name: '业务领域', 
                    levels: ['数字化战略', '业务架构', '数据架构', '技术架构'] 
                }
            },
            nodes: [
                // X=0 战略规划层
                { id: 'strategy-1', title: '数字化转型战略', position: { x: 0, y: 0, z: 0 }, 
                  description: '制定企业数字化转型的总体战略和规划，包括技术路线、组织变革、业务创新等', 
                  tags: ['战略', '规划', '转型'] },
                { id: 'strategy-2', title: '云原生技术选型', position: { x: 0, y: 1, z: 0 }, 
                  description: '评估和选择适合企业需求的云原生技术栈，包括容器、微服务、DevOps等', 
                  tags: ['技术选型', '云原生', '评估'] },
                { id: 'strategy-3', title: '企业架构规划', position: { x: 0, y: 2, z: 0 }, 
                  description: '基于TOGAF等框架进行企业架构规划设计，涵盖业务、数据、应用、技术架构', 
                  tags: ['企业架构', 'TOGAF', '规划'] },
                { id: 'strategy-4', title: '数据治理策略', position: { x: 0, y: 3, z: 0 }, 
                  description: '制定数据治理框架和策略，确保数据质量、安全性和合规性', 
                  tags: ['数据治理', '策略', '合规'] },

                // X=1 架构设计层
                { id: 'arch-1', title: '微服务架构设计', position: { x: 1, y: 2, z: 3 }, 
                  description: '设计微服务拆分策略、服务治理、API设计等架构要素', 
                  tags: ['微服务', '架构', '设计'] },
                { id: 'arch-2', title: '数据中台架构', position: { x: 1, y: 2, z: 2 }, 
                  description: '构建企业级数据中台，实现数据资产化和数据服务化', 
                  tags: ['数据中台', '数据架构', '数据治理'] },
                { id: 'arch-3', title: '业务中台设计', position: { x: 1, y: 3, z: 1 }, 
                  description: '设计业务中台架构，实现业务能力的复用和沉淀', 
                  tags: ['业务中台', '能力复用', '架构'] },
                { id: 'arch-4', title: 'API网关设计', position: { x: 1, y: 2, z: 3 }, 
                  description: '设计API网关架构，实现API的统一管理和安全控制', 
                  tags: ['API网关', '安全', '管理'] },
                { id: 'arch-5', title: '云原生架构', position: { x: 1, y: 1, z: 3 }, 
                  description: '设计云原生技术架构，包括容器编排、服务网格等', 
                  tags: ['云原生', '架构', '容器'] },

                // X=2 实施交付层
                { id: 'impl-1', title: 'DevOps流水线', position: { x: 2, y: 1, z: 3 }, 
                  description: '建立持续集成、持续交付的DevOps自动化流水线', 
                  tags: ['DevOps', 'CI/CD', '自动化'] },
                { id: 'impl-2', title: '容器化部署', position: { x: 2, y: 0, z: 3 }, 
                  description: '应用容器化改造和Kubernetes集群部署实施', 
                  tags: ['容器', 'K8s', '部署'] },
                { id: 'impl-3', title: '微服务开发', position: { x: 2, y: 2, z: 3 }, 
                  description: '基于Spring Cloud等框架进行微服务应用开发', 
                  tags: ['微服务', '开发', 'Spring Cloud'] },
                { id: 'impl-4', title: '数据平台实施', position: { x: 2, y: 2, z: 2 }, 
                  description: '数据中台和数据平台的具体实施和部署', 
                  tags: ['数据平台', '实施', '部署'] },
                { id: 'impl-5', title: '安全体系建设', position: { x: 2, y: 1, z: 0 }, 
                  description: '构建全方位的信息安全防护体系', 
                  tags: ['安全', '防护', '体系'] },

                // X=3 运营治理层
                { id: 'ops-1', title: '微服务治理', position: { x: 3, y: 2, z: 3 }, 
                  description: '建立微服务运行监控、限流熔断、服务发现等治理机制', 
                  tags: ['治理', '监控', '微服务'] },
                { id: 'ops-2', title: '数据质量管理', position: { x: 3, y: 2, z: 2 }, 
                  description: '建立数据质量监控、数据血缘、数据标准等管理机制', 
                  tags: ['数据质量', '数据治理', '监控'] },
                { id: 'ops-3', title: '性能监控', position: { x: 3, y: 0, z: 3 }, 
                  description: '构建应用性能监控体系，确保系统稳定运行', 
                  tags: ['监控', '性能', '运维'] },
                { id: 'ops-4', title: '成本优化', position: { x: 3, y: 0, z: 0 }, 
                  description: '云资源成本监控和优化策略实施', 
                  tags: ['成本', '优化', '云资源'] },
                { id: 'ops-5', title: '业务连续性', position: { x: 3, y: 3, z: 0 }, 
                  description: '建立业务连续性和灾备恢复机制', 
                  tags: ['连续性', '灾备', '业务'] },

                // 补充更多知识点
                { id: 'platform-1', title: '云原生平台建设', position: { x: 1, y: 1, z: 3 }, 
                  description: '构建企业级云原生技术平台，提供容器、微服务、DevOps等基础能力', 
                  tags: ['云原生', '平台', '基础设施'] },
                { id: 'platform-2', title: '低代码平台', position: { x: 1, y: 1, z: 1 }, 
                  description: '构建企业级低代码开发平台，提升开发效率', 
                  tags: ['低代码', '平台', '开发'] },
                { id: 'service-1', title: '服务网格', position: { x: 2, y: 2, z: 3 }, 
                  description: '基于Istio等服务网格技术实现微服务治理', 
                  tags: ['服务网格', 'Istio', '治理'] },
                { id: 'service-2', title: 'API管理', position: { x: 3, y: 2, z: 1 }, 
                  description: '建立API全生命周期管理和监控体系', 
                  tags: ['API', '管理', '生命周期'] },
                { id: 'data-1', title: '数据湖建设', position: { x: 1, y: 0, z: 2 }, 
                  description: '构建企业数据湖，实现数据集中存储和管理', 
                  tags: ['数据湖', '存储', '管理'] },
                { id: 'data-2', title: '数据安全', position: { x: 3, y: 0, z: 2 }, 
                  description: '数据安全防护和隐私保护机制', 
                  tags: ['数据安全', '隐私', '防护'] }
            ]
        };

        type DimensionKey = 'x' | 'y' | 'z';
        type DimensionState = Record<DimensionKey, boolean>;
        type NodeType = {
            title: string,
            position: {
                x: number,
                y: number,
                z: number 
            },
            tags: string[],
            description: string
        }
        type SelectedNodeType = NodeType & {
            xLevel: string;
            yLevel: string;
            zLevel: string;
        };

        // Three.js相关变量
        let scene: THREE.Scene;
        let camera: THREE.PerspectiveCamera;
        let renderer: THREE.WebGLRenderer;
        let controls: THREE.OrbitControls;
        let cube, nodes = [];
        let activeDimensions: DimensionState = { x: true, y: true, z: true };
        let selectedLevels = { x: 0, y: 0, z: 0 };
        let currentView = 'cube';
        let container: HTMLDivElement | null = null;
        let selectedNode: SelectedNodeType | null = $state(null);

        // 初始化Three.js场景
        function initThreeJS() {
            // const container = document.getElementById('canvas-container');
            if (!container) {
                console.error("Container element is missing! Initialization failed.");
                return;
            }

            // 创建场景
            scene = new THREE.Scene();
            scene.background = new THREE.Color(0xf0f0f0);

            // 创建相机
            camera = new THREE.PerspectiveCamera(75, container.clientWidth / container.clientHeight, 0.1, 1000);
            camera.position.set(5, 5, 5);

            // 创建渲染器
            renderer = new THREE.WebGLRenderer({ antialias: true });
            renderer.setSize(container.clientWidth, container.clientHeight);
            container.appendChild(renderer.domElement);

            // 添加轨道控制器
            controls = new THREE.OrbitControls(camera, renderer.domElement);
            controls.enableDamping = true;
            controls.dampingFactor = 0.05;

            // 添加灯光
            const ambientLight = new THREE.AmbientLight(0x404040, 0.6);
            scene.add(ambientLight);

            const directionalLight = new THREE.DirectionalLight(0xffffff, 0.8);
            directionalLight.position.set(1, 1, 1);
            scene.add(directionalLight);

            // 创建立方体框架
            createCubeFrame();

            // 创建知识节点
            createKnowledgeNodes();

            // 开始动画循环
            animate();

            // 窗口大小调整
            window.addEventListener('resize', onWindowResize);
        }

        // 创建立方体框架
        function createCubeFrame() {
            // 创建外部大立方体框架
            const geometry = new THREE.BoxGeometry(4, 4, 4);
            const edges = new THREE.EdgesGeometry(geometry);
            const line = new THREE.LineSegments(edges, new THREE.LineBasicMaterial({ color: 0x000000, linewidth: 2 }));
            scene.add(line);

            // 创建内部网格线 - 4x4x4的小立方体
            const gridMaterial = new THREE.LineBasicMaterial({ color: 0xcccccc, linewidth: 1 });

            // X轴方向的网格线
            for (let i = 1; i < 4; i++) {
                const x = (i / 4) * 4 - 2;

                // YZ平面
                const yzGeometry = new THREE.BufferGeometry().setFromPoints([
                    new THREE.Vector3(x, -2, -2),
                    new THREE.Vector3(x, -2, 2),
                    new THREE.Vector3(x, 2, 2),
                    new THREE.Vector3(x, 2, -2),
                    new THREE.Vector3(x, -2, -2)
                ]);
                scene.add(new THREE.Line(yzGeometry, gridMaterial));
            }

            // Y轴方向的网格线
            for (let i = 1; i < 4; i++) {
                const y = (i / 4) * 4 - 2;

                // XZ平面
                const xzGeometry = new THREE.BufferGeometry().setFromPoints([
                    new THREE.Vector3(-2, y, -2),
                    new THREE.Vector3(-2, y, 2),
                    new THREE.Vector3(2, y, 2),
                    new THREE.Vector3(2, y, -2),
                    new THREE.Vector3(-2, y, -2)
                ]);
                scene.add(new THREE.Line(xzGeometry, gridMaterial));
            }

            // Z轴方向的网格线
            for (let i = 1; i < 4; i++) {
                const z = (i / 4) * 4 - 2;

                // XY平面
                const xyGeometry = new THREE.BufferGeometry().setFromPoints([
                    new THREE.Vector3(-2, -2, z),
                    new THREE.Vector3(-2, 2, z),
                    new THREE.Vector3(2, 2, z),
                    new THREE.Vector3(2, -2, z),
                    new THREE.Vector3(-2, -2, z)
                ]);
                scene.add(new THREE.Line(xyGeometry, gridMaterial));
            }

            // 添加坐标轴标签
            addAxisLabels();
        }

        // 添加坐标轴标签
        function addAxisLabels() {
            // 这里可以添加X、Y、Z轴的文本标签
            // 简化实现，实际项目中可以使用CSS3DRenderer
        }

        // 创建知识节点
        function createKnowledgeNodes() {
            knowledgeSystem.nodes.forEach(node => {
                const geometry = new THREE.SphereGeometry(0.15, 16, 16);
                const material = new THREE.MeshPhongMaterial({ 
                    color: getNodeColor(node),
                    emissive: 0x072534,
                    shininess: 100
                });

                const sphere = new THREE.Mesh(geometry, material);
                sphere.position.set(
                    node.position.x - 1.5,
                    node.position.y - 1.5, 
                    node.position.z - 1.5
                );
                sphere.userData = node;

                scene.add(sphere);
                nodes.push(sphere);

                // 添加点击事件
                sphere.userData.onClick = () => showNodeInfo(node);
            });
        }

        // 根据节点位置获取颜色
        function getNodeColor(node) {
            const colors = [
                0xff6b6b, // 红色 - 战略规划
                0x4ecdc4, // 青色 - 架构设计  
                0x45b7d1, // 蓝色 - 实施交付
                0x96ceb4  // 绿色 - 运营治理
            ];
            return colors[node.position.x] || 0xaaaaaa;
        }

        // 显示节点信息
        function showNodeInfo(node: NodeType) {
            const detailsContainer = document.getElementById('node-details');
            const xLevel = knowledgeSystem.dimensions.x.levels[node.position.x];
            const yLevel = knowledgeSystem.dimensions.y.levels[node.position.y];
            const zLevel = knowledgeSystem.dimensions.z.levels[node.position.z];

            selectedNode = {
                ...node,
                xLevel,
                yLevel,
                zLevel
            }
        }

        // 切换维度显示
        function toggleDimension(dimension: DimensionKey) {
            const btn = event.target;
            activeDimensions[dimension] = !activeDimensions[dimension];
            btn.textContent = activeDimensions[dimension] ? '开启' : '关闭';
            btn.classList.toggle('active', activeDimensions[dimension]);

            updateView();
        }

        // 选择维度层级
        function selectLevel(dimension, level) {
            selectedLevels[dimension] = level;

            // 更新UI
            const levels = document.getElementById(`${dimension}-levels`).children;
            for (let i = 0; i < levels.length; i++) {
                levels[i].classList.toggle('active', i === level);
            }

            updateView();
        }

        // 更新视图
        function updateView() {
            const activeCount = Object.values(activeDimensions).filter(v => v).length;

            if (activeCount === 3) {
                // 3D立方体视图
                switchView('cube');
            } else {
                // 2D矩阵视图
                switchView('matrix');
                updateMatrixView();
            }
        }

        // 切换视图
        function switchView(view) {
            currentView = view;

            const cubeBtn = document.getElementById('cube-view-btn');
            const matrixBtn = document.getElementById('matrix-view-btn');
            const canvasContainer = document.getElementById('canvas-container');
            const matrixView = document.getElementById('matrix-view');

            if (view === 'cube') {
                cubeBtn.classList.add('active');
                matrixBtn.classList.remove('active');
                canvasContainer.style.display = 'block';
                matrixView.style.display = 'none';
            } else {
                cubeBtn.classList.remove('active');
                matrixBtn.classList.add('active');
                canvasContainer.style.display = 'none';
                matrixView.style.display = 'block';
            }
        }

        // 更新矩阵视图
        function updateMatrixView() {
            const matrixGrid = document.getElementById('matrix-grid');
            matrixGrid.innerHTML = '';

            // 确定当前活跃的维度
            const activeDims = Object.entries(activeDimensions)
                .filter(([_, isActive]) => isActive)
                .map(([dim]) => dim);

            if (activeDims.length !== 2) return;

            const [dim1, dim2] = activeDims;
            const dim1Levels = knowledgeSystem.dimensions[dim1].levels;
            const dim2Levels = knowledgeSystem.dimensions[dim2].levels;

            // 更新矩阵视图标题
            const matrixTitle = document.querySelector('#matrix-view h3');
            matrixTitle.textContent = `知识矩阵视图 - ${knowledgeSystem.dimensions[dim1].name} × ${knowledgeSystem.dimensions[dim2].name}`;

            // 设置网格布局
            matrixGrid.style.gridTemplateColumns = `repeat(${dim1Levels.length}, 1fr)`;
            matrixGrid.style.gridTemplateRows = `repeat(${dim2Levels.length}, 1fr)`;

            // 创建矩阵单元格
            for (let i = 0; i < dim1Levels.length; i++) {
                for (let j = 0; j < dim2Levels.length; j++) {
                    const cell = document.createElement('div');
                    cell.className = 'matrix-cell';

                    // 查找该单元格中的节点
                    const nodesInCell = knowledgeSystem.nodes.filter(node => {
                        const pos = {};
                        pos[dim1] = i;
                        pos[dim2] = j;

                        // 检查第三个维度的选择
                        const inactiveDim = Object.keys(activeDimensions).find(dim => !activeDimensions[dim]);
                        if (inactiveDim) {
                            pos[inactiveDim] = selectedLevels[inactiveDim];
                        }

                        return node.position.x === pos.x && node.position.y === pos.y && node.position.z === pos.z;
                    });

                    if (nodesInCell.length > 0) {
                        cell.innerHTML = `
                            <h4>${nodesInCell[0].title}</h4>
                            <p>${nodesInCell.length} 个知识点</p>
                        `;
                        cell.onclick = () => showNodeInfo(nodesInCell[0]);
                    } else {
                        cell.innerHTML = '<p>无内容</p>';
                        cell.style.background = '#f8f9fa';
                        cell.style.color = '#6c757d';
                    }

                    matrixGrid.appendChild(cell);
                }
            }
        }

        // 动画循环
        function animate() {
            requestAnimationFrame(animate);
            controls.update();
            renderer.render(scene, camera);
        }

        // 窗口大小调整
        function onWindowResize() {
            // const container = document.getElementById('canvas-container');
            camera.aspect = container.clientWidth / container.clientHeight;
            camera.updateProjectionMatrix();
            renderer.setSize(container.clientWidth, container.clientHeight);
        }

        // 初始化
        document.addEventListener('DOMContentLoaded', () => {
            initThreeJS();

            // 添加点击事件监听
            renderer.domElement.addEventListener('click', onCanvasClick);
        });

        // 画布点击事件
        function onCanvasClick(event) {
            const mouse = new THREE.Vector2();
            const rect = renderer.domElement.getBoundingClientRect();

            mouse.x = ((event.clientX - rect.left) / rect.width) * 2 - 1;
            mouse.y = -((event.clientY - rect.top) / rect.height) * 2 + 1;

            const raycaster = new THREE.Raycaster();
            raycaster.setFromCamera(mouse, camera);

            const intersects = raycaster.intersectObjects(nodes);

            if (intersects.length > 0) {
                const node = intersects[0].object.userData;
                if (node.onClick) {
                    node.onClick();
                }
            }
        }
</script>
<!--
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>数字化知识立方体 - 多维知识体系可视化</title>
    <script src="https://cdn.jsdelivr.net/npm/three@0.132.2/build/three.min.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/three@0.132.2/examples/js/controls/OrbitControls.js"></script>
-->

<div class="container" bind:this={container}>
        <!-- 左侧控制面板 -->
        <div class="control-panel">
            <h1>数字化知识立方体</h1>

            <!-- X轴维度：生命周期 -->
            <div class="dimension-control">
                <div class="dimension-title">
                    <span>X轴: 生命周期阶段</span>
                    <button class="toggle-btn active" onclick="toggleDimension('x')">开启</button>
                </div>
                <div class="levels" id="x-levels">
                    <div class="level active" onclick="selectLevel('x', 0)">战略规划</div>
                    <div class="level" onclick="selectLevel('x', 1)">架构设计</div>
                    <div class="level" onclick="selectLevel('x', 2)">实施交付</div>
                    <div class="level" onclick="selectLevel('x', 3)">运营治理</div>
                </div>
            </div>

            <!-- Y轴维度：架构层次 -->
            <div class="dimension-control">
                <div class="dimension-title">
                    <span>Y轴: 架构层次</span>
                    <button class="toggle-btn active" onclick="toggleDimension('y')">开启</button>
                </div>
                <div class="levels" id="y-levels">
                    <div class="level active" onclick="selectLevel('y', 0)">基础资源</div>
                    <div class="level" onclick="selectLevel('y', 1)">平台能力</div>
                    <div class="level" onclick="selectLevel('y', 2)">技术服务</div>
                    <div class="level" onclick="selectLevel('y', 3)">业务应用</div>
                </div>
            </div>

            <!-- Z轴维度：业务领域 -->
            <div class="dimension-control">
                <div class="dimension-title">
                    <span>Z轴: 业务领域</span>
                    <button class="toggle-btn active" onclick="toggleDimension('z')">开启</button>
                </div>
                <div class="levels" id="z-levels">
                    <div class="level active" onclick="selectLevel('z', 0)">数字化战略</div>
                    <div class="level" onclick="selectLevel('z', 1)">业务架构</div>
                    <div class="level" onclick="selectLevel('z', 2)">数据架构</div>
                    <div class="level" onclick="selectLevel('z', 3)">技术架构</div>
                </div>
            </div>
        </div>

        <!-- 中间可视化区域 -->
        <div class="visualization-area">
            <div class="view-toggle">
                <button id="cube-view-btn" class="active" onclick="switchView('cube')">3D立方体</button>
                <button id="matrix-view-btn" onclick="switchView('matrix')">2D矩阵</button>
            </div>

            <div id="canvas-container"></div>
            <div id="matrix-view" class="matrix-view">
                <h3>知识矩阵视图</h3>
                <div id="matrix-grid" class="matrix-grid"></div>
            </div>
        </div>

        <!-- 右侧信息面板 -->
        <div class="info-panel">
            <h2>知识节点详情</h2>
            <div id="node-details">
                {#if selectedNode}
                    <div class="node-info">
                        <h3>{selectedNode.title}</h3>
            
                        <div class="position-info">
                            <strong>位置信息：</strong><br>
                            X轴({knowledgeSystem.dimensions.x.name}):{selectedNode.xLevel}<br>
                            Y轴({knowledgeSystem.dimensions.y.name}):{selectedNode.yLevel}<br>
                            Z轴({knowledgeSystem.dimensions.z.name}):{selectedNode.zLevel}
                        </div>
            
                        <p><strong>描述：</strong>{selectedNode.description}</p>
            
                        <div class="tags">
                            {#each selectedNode.tags as tag}
                                <span class="tag">{tag}</span>
                            {/each}
                        </div>
                    </div>
                {:else}
                    <p>点击立方体中的节点查看详细信息。</p>
                {/if}
            </div>
        </div>
</div>

<style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: 'Microsoft YaHei', Arial, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: #333;
            overflow: hidden;
        }

        .container {
            display: flex;
            height: 100vh;
            padding: 20px;
            gap: 20px;
        }

        .control-panel {
            width: 300px;
            background: rgba(255, 255, 255, 0.95);
            border-radius: 15px;
            padding: 20px;
            box-shadow: 0 8px 32px rgba(0, 0, 0, 0.1);
            backdrop-filter: blur(10px);
        }

        .visualization-area {
            flex: 1;
            background: rgba(255, 255, 255, 0.1);
            border-radius: 15px;
            position: relative;
            overflow: hidden;
        }

        .info-panel {
            width: 350px;
            background: rgba(255, 255, 255, 0.95);
            border-radius: 15px;
            padding: 20px;
            box-shadow: 0 8px 32px rgba(0, 0, 0, 0.1);
            backdrop-filter: blur(10px);
            overflow-y: auto;
        }

        h1 {
            color: #2c3e50;
            margin-bottom: 20px;
            text-align: center;
            font-size: 24px;
        }

        .dimension-control {
            margin-bottom: 20px;
            padding: 15px;
            background: #f8f9fa;
            border-radius: 10px;
            border-left: 4px solid #3498db;
        }

        .dimension-title {
            font-weight: bold;
            margin-bottom: 10px;
            color: #2c3e50;
            display: flex;
            align-items: center;
            justify-content: space-between;
        }

        .toggle-btn {
            background: #e74c3c;
            color: white;
            border: none;
            padding: 5px 10px;
            border-radius: 5px;
            cursor: pointer;
            font-size: 12px;
        }

        .toggle-btn.active {
            background: #27ae60;
        }

        .levels {
            display: flex;
            flex-direction: column;
            gap: 5px;
        }

        .level {
            padding: 8px 12px;
            background: white;
            border-radius: 6px;
            border: 1px solid #e1e8ed;
            cursor: pointer;
            transition: all 0.3s ease;
        }

        .level:hover {
            background: #3498db;
            color: white;
            transform: translateX(5px);
        }

        .level.active {
            background: #2980b9;
            color: white;
            border-color: #2980b9;
        }

        #canvas-container {
            width: 100%;
            height: 100%;
            background: rgba(255, 255, 255, 0.05);
            border-radius: 10px;
        }

        .node-info {
            background: white;
            padding: 15px;
            border-radius: 10px;
            margin-bottom: 15px;
            border-left: 4px solid #9b59b6;
        }

        .node-info h3 {
            color: #2c3e50;
            margin-bottom: 10px;
        }

        .node-info p {
            color: #7f8c8d;
            line-height: 1.6;
            margin-bottom: 8px;
        }

        .position-info {
            background: #ecf0f1;
            padding: 8px 12px;
            border-radius: 6px;
            margin-bottom: 10px;
            font-size: 14px;
        }

        .position-info strong {
            color: #2c3e50;
        }

        .tags {
            display: flex;
            flex-wrap: wrap;
            gap: 5px;
            margin-top: 10px;
        }

        .tag {
            background: #ecf0f1;
            padding: 3px 8px;
            border-radius: 12px;
            font-size: 12px;
            color: #34495e;
        }

        .matrix-view {
            display: none;
            width: 100%;
            height: 100%;
            background: white;
            border-radius: 10px;
            padding: 20px;
            overflow: auto;
        }

        .matrix-grid {
            display: grid;
            gap: 10px;
            margin-top: 20px;
        }

        .matrix-cell {
            background: #f8f9fa;
            border: 2px solid #e9ecef;
            border-radius: 8px;
            padding: 15px;
            cursor: pointer;
            transition: all 0.3s ease;
        }

        .matrix-cell:hover {
            background: #3498db;
            color: white;
            transform: scale(1.05);
        }

        .matrix-cell h4 {
            margin-bottom: 5px;
            font-size: 14px;
        }

        .matrix-cell p {
            font-size: 12px;
            color: #6c757d;
        }

        .view-toggle {
            position: absolute;
            top: 20px;
            right: 20px;
            background: rgba(255, 255, 255, 0.9);
            padding: 10px 15px;
            border-radius: 25px;
            box-shadow: 0 4px 15px rgba(0, 0, 0, 0.1);
            z-index: 100;
        }

        .view-toggle button {
            background: #3498db;
            color: white;
            border: none;
            padding: 8px 16px;
            border-radius: 20px;
            cursor: pointer;
            margin: 0 5px;
            transition: all 0.3s ease;
        }

        .view-toggle button.active {
            background: #e74c3c;
        }
</style>

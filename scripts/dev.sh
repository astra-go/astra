#!/usr/bin/env bash
# Astra Development Environment Management Script
# Usage: ./scripts/dev.sh [command] [options]

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
COMPOSE_FILE="deploy/docker-compose.dev.yml"
PROJECT_NAME="astra-dev"

# Helper functions
print_header() {
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

check_docker() {
    if ! command -v docker &> /dev/null; then
        print_error "Docker is not installed. Please install Docker first."
        exit 1
    fi

    if ! docker info &> /dev/null; then
        print_error "Docker daemon is not running. Please start Docker."
        exit 1
    fi
}

check_docker_compose() {
    if ! docker compose version &> /dev/null; then
        print_error "Docker Compose is not installed or not available."
        exit 1
    fi
}

# Command: start
cmd_start() {
    local profile="${1:-minimal}"

    print_header "Starting Astra Development Environment"

    check_docker
    check_docker_compose

    case "$profile" in
        minimal)
            print_info "Starting minimal setup (PostgreSQL + Redis)..."
            docker compose -f "$COMPOSE_FILE" -p "$PROJECT_NAME" up -d postgres redis
            ;;
        observability)
            print_info "Starting with observability stack (PostgreSQL + Redis + Prometheus + Grafana + Jaeger)..."
            docker compose -f "$COMPOSE_FILE" -p "$PROJECT_NAME" --profile observability up -d
            ;;
        full)
            print_info "Starting full setup (all services)..."
            docker compose -f "$COMPOSE_FILE" -p "$PROJECT_NAME" --profile full up -d
            ;;
        *)
            print_error "Unknown profile: $profile"
            echo "Available profiles: minimal, observability, full"
            exit 1
            ;;
    esac

    echo ""
    print_success "Environment started successfully!"
    echo ""
    cmd_status
}

# Command: stop
cmd_stop() {
    print_header "Stopping Astra Development Environment"

    check_docker
    check_docker_compose

    docker compose -f "$COMPOSE_FILE" -p "$PROJECT_NAME" stop

    print_success "Environment stopped successfully!"
}

# Command: down
cmd_down() {
    local remove_volumes="${1:-no}"

    print_header "Shutting Down Astra Development Environment"

    check_docker
    check_docker_compose

    if [[ "$remove_volumes" == "--volumes" || "$remove_volumes" == "-v" ]]; then
        print_warning "This will remove all data volumes!"
        read -p "Are you sure? (yes/no): " confirm
        if [[ "$confirm" == "yes" ]]; then
            docker compose -f "$COMPOSE_FILE" -p "$PROJECT_NAME" down -v
            print_success "Environment shut down and volumes removed!"
        else
            print_info "Operation cancelled."
            exit 0
        fi
    else
        docker compose -f "$COMPOSE_FILE" -p "$PROJECT_NAME" down
        print_success "Environment shut down (data preserved)!"
    fi
}

# Command: restart
cmd_restart() {
    local service="$1"

    print_header "Restarting Services"

    check_docker
    check_docker_compose

    if [[ -n "$service" ]]; then
        print_info "Restarting service: $service"
        docker compose -f "$COMPOSE_FILE" -p "$PROJECT_NAME" restart "$service"
    else
        print_info "Restarting all services..."
        docker compose -f "$COMPOSE_FILE" -p "$PROJECT_NAME" restart
    fi

    print_success "Restart completed!"
}

# Command: status
cmd_status() {
    print_header "Service Status"

    check_docker

    docker compose -f "$COMPOSE_FILE" -p "$PROJECT_NAME" ps

    echo ""
    print_header "Service Endpoints"

    # Core services
    if docker ps --filter "name=astra-postgres" --filter "status=running" -q | grep -q .; then
        print_success "PostgreSQL:    postgresql://astra_dev:dev123@localhost:5432/astra_dev"
    fi

    if docker ps --filter "name=astra-redis" --filter "status=running" -q | grep -q .; then
        print_success "Redis:         redis://:dev123@localhost:6379"
    fi

    # Optional services
    if docker ps --filter "name=astra-mysql" --filter "status=running" -q | grep -q .; then
        print_success "MySQL:         mysql://astra_dev:dev123@localhost:3306/astra_dev"
    fi

    if docker ps --filter "name=astra-mongodb" --filter "status=running" -q | grep -q .; then
        print_success "MongoDB:       mongodb://astra_dev:dev123@localhost:27017/astra_dev"
    fi

    if docker ps --filter "name=astra-kafka" --filter "status=running" -q | grep -q .; then
        print_success "Kafka:         localhost:9092"
    fi

    if docker ps --filter "name=astra-rabbitmq" --filter "status=running" -q | grep -q .; then
        print_success "RabbitMQ:      amqp://astra_dev:dev123@localhost:5672"
        print_success "RabbitMQ UI:   http://localhost:15672 (astra_dev/dev123)"
    fi

    if docker ps --filter "name=astra-nats" --filter "status=running" -q | grep -q .; then
        print_success "NATS:          nats://localhost:4222"
    fi

    if docker ps --filter "name=astra-elasticsearch" --filter "status=running" -q | grep -q .; then
        print_success "Elasticsearch: http://localhost:9200"
    fi

    # Observability
    if docker ps --filter "name=astra-prometheus" --filter "status=running" -q | grep -q .; then
        print_success "Prometheus:    http://localhost:9090"
    fi

    if docker ps --filter "name=astra-grafana" --filter "status=running" -q | grep -q .; then
        print_success "Grafana:       http://localhost:3000 (admin/admin)"
    fi

    if docker ps --filter "name=astra-jaeger" --filter "status=running" -q | grep -q .; then
        print_success "Jaeger UI:     http://localhost:16686"
    fi

    # Service discovery
    if docker ps --filter "name=astra-consul" --filter "status=running" -q | grep -q .; then
        print_success "Consul:        http://localhost:8500"
    fi

    if docker ps --filter "name=astra-etcd" --filter "status=running" -q | grep -q .; then
        print_success "etcd:          http://localhost:2379"
    fi
}

# Command: logs
cmd_logs() {
    local service="$1"
    local follow="${2:-no}"

    check_docker
    check_docker_compose

    if [[ -n "$service" ]]; then
        if [[ "$follow" == "-f" || "$follow" == "--follow" ]]; then
            docker compose -f "$COMPOSE_FILE" -p "$PROJECT_NAME" logs -f "$service"
        else
            docker compose -f "$COMPOSE_FILE" -p "$PROJECT_NAME" logs --tail=100 "$service"
        fi
    else
        if [[ "$follow" == "-f" || "$follow" == "--follow" ]]; then
            docker compose -f "$COMPOSE_FILE" -p "$PROJECT_NAME" logs -f
        else
            docker compose -f "$COMPOSE_FILE" -p "$PROJECT_NAME" logs --tail=100
        fi
    fi
}

# Command: clean
cmd_clean() {
    print_header "Cleaning Up Docker Resources"

    check_docker

    print_warning "This will remove:"
    echo "  - All stopped containers"
    echo "  - All unused networks"
    echo "  - All dangling images"
    echo "  - Build cache"
    echo ""
    read -p "Continue? (yes/no): " confirm

    if [[ "$confirm" == "yes" ]]; then
        docker system prune -f
        print_success "Cleanup completed!"
    else
        print_info "Operation cancelled."
    fi
}

# Command: reset
cmd_reset() {
    print_header "Resetting Development Environment"

    print_warning "This will:"
    echo "  - Stop all services"
    echo "  - Remove all containers and networks"
    echo "  - Remove all data volumes (ALL DATA WILL BE LOST)"
    echo ""
    read -p "Are you absolutely sure? (yes/no): " confirm

    if [[ "$confirm" == "yes" ]]; then
        cmd_down --volumes
        print_success "Environment reset completed!"
        echo ""
        print_info "Run './scripts/dev.sh start' to recreate the environment"
    else
        print_info "Operation cancelled."
    fi
}

# Command: health
cmd_health() {
    print_header "Health Check"

    check_docker

    local all_healthy=true

    # Check each service
    for container in $(docker ps --filter "name=astra-" --format "{{.Names}}"); do
        health_status=$(docker inspect --format='{{.State.Health.Status}}' "$container" 2>/dev/null || echo "none")

        if [[ "$health_status" == "healthy" ]]; then
            print_success "$container: healthy"
        elif [[ "$health_status" == "none" ]]; then
            print_info "$container: no health check"
        elif [[ "$health_status" == "starting" ]]; then
            print_warning "$container: starting..."
            all_healthy=false
        else
            print_error "$container: unhealthy"
            all_healthy=false
        fi
    done

    echo ""
    if $all_healthy; then
        print_success "All services are healthy!"
    else
        print_warning "Some services are not healthy yet. Run 'health' again in a few seconds."
    fi
}

# Command: help
cmd_help() {
    cat << EOF
Astra Development Environment Management Script

USAGE:
    ./scripts/dev.sh <command> [options]

COMMANDS:
    start [profile]       Start the development environment
                         Profiles: minimal (default), observability, full
    stop                 Stop all services (preserves data)
    down [-v|--volumes]  Stop and remove containers (optionally remove volumes)
    restart [service]    Restart all services or a specific service
    status               Show service status and endpoints
    logs [service] [-f]  Show logs (optionally follow)
    health               Check health status of all services
    clean                Clean up unused Docker resources
    reset                Reset environment (removes all data)
    help                 Show this help message

EXAMPLES:
    # Start minimal environment (PostgreSQL + Redis)
    ./scripts/dev.sh start

    # Start with observability stack
    ./scripts/dev.sh start observability

    # Start all services
    ./scripts/dev.sh start full

    # Check service status
    ./scripts/dev.sh status

    # View logs for a specific service
    ./scripts/dev.sh logs postgres

    # Follow logs in real-time
    ./scripts/dev.sh logs postgres -f

    # Restart a service
    ./scripts/dev.sh restart redis

    # Stop environment (preserve data)
    ./scripts/dev.sh stop

    # Complete reset (removes all data)
    ./scripts/dev.sh reset

PROFILES:
    minimal        PostgreSQL + Redis (default)
    observability  Minimal + Prometheus + Grafana + Jaeger
    full           All services including Kafka, MongoDB, Elasticsearch, etc.

For more information, see: deploy/README.md
EOF
}

# Main script
main() {
    local command="${1:-help}"
    shift || true

    case "$command" in
        start)
            cmd_start "$@"
            ;;
        stop)
            cmd_stop
            ;;
        down)
            cmd_down "$@"
            ;;
        restart)
            cmd_restart "$@"
            ;;
        status)
            cmd_status
            ;;
        logs)
            cmd_logs "$@"
            ;;
        health)
            cmd_health
            ;;
        clean)
            cmd_clean
            ;;
        reset)
            cmd_reset
            ;;
        help|--help|-h)
            cmd_help
            ;;
        *)
            print_error "Unknown command: $command"
            echo ""
            cmd_help
            exit 1
            ;;
    esac
}

main "$@"
